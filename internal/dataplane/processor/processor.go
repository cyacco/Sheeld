package processor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/dataplane/alert"
	"github.com/cyacco/Sheeld/internal/dataplane/backendconfig"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/llm"
	"github.com/cyacco/Sheeld/internal/shared/metrics"
	"github.com/cyacco/Sheeld/internal/shared/middleware"
	"github.com/cyacco/Sheeld/internal/shared/transform"
)

// Result is the full result of a proxy request.
type Result struct {
	// Status is "pass" or "rejected".
	Status string `json:"status"`

	// Phase indicates where rejection happened ("input" or "output"). Empty on pass.
	Phase string `json:"phase,omitempty"`

	// LLMResponse is the chat completion response (nil if input was rejected).
	LLMResponse *llm.ChatResponse `json:"llm_response,omitempty"`

	// GuardResults contains per-phase guard evaluation results.
	GuardResults map[string]*guard.EngineResult `json:"guard_results"`

	// Transforms records the input transformer chain outcome (nil when the
	// source has no input transformers).
	Transforms *transform.ChainResult `json:"transforms,omitempty"`

	// OutputTransforms records the output transformer chain outcome, run on
	// the LLM response before output guards (nil when the source has no
	// output transformers or the request was rejected at input).
	OutputTransforms *transform.ChainResult `json:"output_transforms,omitempty"`

	// LatencyMs is the total wall-clock time for the request.
	LatencyMs int64 `json:"latency_ms"`
}

// AuditSink receives completed proxy results for asynchronous recording.
// usage/model are nil/"" when no LLM call was made (e.g. an input-guard
// rejection short-circuits before the provider is called).
type AuditSink interface {
	Record(orgID, sourceID uuid.UUID, inputText string, guardResults map[string]*guard.EngineResult, transforms, outputTransforms *transform.ChainResult, overallResult string, latencyMs int64, usage *llm.Usage, model string)
}

// AlertSink receives rejection events for asynchronous alert delivery.
// Implementations must not block.
type AlertSink interface {
	Notify(event alert.Event)
}

// Processor runs the proxy stages: input guards → LLM call → output guards.
// All configuration comes from the in-memory store; no I/O beyond the LLM
// call happens on the request path.
type Processor struct {
	store     *backendconfig.Store
	engine    *guard.Engine
	llmClient *llm.Client
	audit     AuditSink
	alerts    AlertSink
}

// NewProcessor creates a processor. audit may be nil to disable audit
// logging; alerts may be nil to disable rejection alerting.
func NewProcessor(store *backendconfig.Store, engine *guard.Engine, llmClient *llm.Client, audit AuditSink, alerts AlertSink) *Processor {
	return &Processor{store: store, engine: engine, llmClient: llmClient, audit: audit, alerts: alerts}
}

// alert fires a rejection event carrying the phase's failing (non-shadow)
// guards. No-op when alerting is disabled.
func (p *Processor) alert(ctx context.Context, orgID uuid.UUID, sourceRoute, phase string, res *guard.EngineResult) {
	if p.alerts == nil || res == nil {
		return
	}
	var failed []alert.FailedGuard
	for _, r := range res.Results {
		if !r.Passed && !r.Shadow {
			failed = append(failed, alert.FailedGuard{Name: r.GuardName, Type: r.GuardType, Message: r.Message})
		}
	}
	p.alerts.Notify(alert.Event{
		OrganizationID: orgID,
		SourceRoute:    sourceRoute,
		Phase:          phase,
		RequestID:      requestID(ctx),
		FailedGuards:   failed,
		Timestamp:      time.Now(),
	})
}

// requestID extracts the request ID from the context.
func requestID(ctx context.Context) string {
	if id, ok := ctx.Value(middleware.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// Execute runs the full proxy flow for a given source and chat request.
func (p *Processor) Execute(ctx context.Context, orgID uuid.UUID, sourceRoute string, chatReq *llm.ChatRequest) (result *Result, err error) {
	start := time.Now()
	log := slog.With("request_id", requestID(ctx), "source", sourceRoute)
	log.Info("proxy request started")

	// Record proxy outcome metrics once, regardless of exit path.
	defer func() {
		metrics.ProxyDuration.Observe(time.Since(start).Seconds())
		switch {
		case err != nil:
			metrics.ProxyRequests.WithLabelValues("error", "").Inc()
		case result != nil && result.Status == "rejected":
			metrics.ProxyRequests.WithLabelValues("rejected", result.Phase).Inc()
		default:
			metrics.ProxyRequests.WithLabelValues("pass", "").Inc()
		}
	}()

	source, ok := p.store.LookupSource(orgID, sourceRoute)
	if !ok {
		return nil, fmt.Errorf("source not found: %q", sourceRoute)
	}
	if !source.Enabled {
		return nil, fmt.Errorf("source %q is disabled", sourceRoute)
	}

	// Per-request transformer state, shared by the input and output chains
	// (e.g. reversible anonymization mappings). Never logged or audited.
	ctx = transform.WithState(ctx)

	inputEval := guard.EvalConfig{Criteria: guard.PassCriteria(source.InputPassCriteria)}
	if source.InputPassThreshold != nil {
		inputEval.Threshold = *source.InputPassThreshold
	}
	outputEval := guard.EvalConfig{Criteria: guard.PassCriteria(source.OutputPassCriteria)}
	if source.OutputPassThreshold != nil {
		outputEval.Threshold = *source.OutputPassThreshold
	}

	// Transformers: sequential rewrites of the whole messages array. The
	// transformed request is what input guards and the LLM see. Errors are
	// fail-closed (proxy error) unless the transformer is fail-open.
	var transforms *transform.ChainResult
	if len(source.InputTransformers) > 0 {
		tCtx := guard.WithCallMeta(ctx, guard.CallMeta{Phase: "input", SourceRoute: source.Route})
		msgs, chain, err := transform.ApplyAll(tCtx, source.InputTransformers, chatReq.Messages)
		transforms = chain
		if err != nil {
			return nil, fmt.Errorf("running transformers: %w", err)
		}
		chatReq.Messages = msgs
		log.Info("transformers completed",
			"transformer_count", len(source.InputTransformers),
			"changed", chain.Changed,
			"latency_ms", chain.TotalDurationMs,
		)
	}

	inputText := llm.ExtractInputText(chatReq)

	// Input guards
	guardResults := make(map[string]*guard.EngineResult)
	if len(source.InputGuards) > 0 {
		guardStart := time.Now()
		inputCtx := guard.WithCallMeta(ctx, guard.CallMeta{
			Phase:           "input",
			SourceRoute:     source.Route,
			AllMessagesText: llm.SerializeMessages(chatReq.Messages),
		})
		inputResult, err := p.engine.Run(inputCtx, source.InputGuards, inputText, inputEval)
		metrics.GuardDuration.WithLabelValues("input").Observe(time.Since(guardStart).Seconds())
		if err != nil {
			return nil, fmt.Errorf("running input guards: %w", err)
		}
		log.Info("input guards completed",
			"guard_count", len(source.InputGuards),
			"passed", inputResult.Passed,
			"latency_ms", time.Since(guardStart).Milliseconds(),
		)
		guardResults["input"] = inputResult

		// Reject at input: no LLM call, tokens saved
		if !inputResult.Passed {
			result = &Result{
				Status:       "rejected",
				Phase:        "input",
				GuardResults: guardResults,
				Transforms:   transforms,
				LatencyMs:    time.Since(start).Milliseconds(),
			}
			log.Info("request rejected at input phase", "total_latency_ms", result.LatencyMs)
			p.record(source, orgID, inputText, guardResults, transforms, nil, "fail", result.LatencyMs, nil, "")
			p.alert(ctx, orgID, source.Route, "input", inputResult)
			return result, nil
		}
	}

	// Call LLM via gateway, overriding the model with the source's config
	chatReq.Model = source.LLMModel

	llmStart := time.Now()
	log.Info("calling LLM gateway", "model", chatReq.Model)

	// A per-source base URL sends this source's traffic directly to its own
	// OpenAI-compatible endpoint; empty falls back to the configured gateway.
	chatResp, err := p.llmClient.ChatCompletionAt(ctx, source.LLMBaseURL, source.LLMAPIKey, chatReq)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}
	log.Info("LLM call completed", "latency_ms", time.Since(llmStart).Milliseconds())

	// Output transformers: rewrite the response before output guards, so
	// guards validate the text the client will actually receive. Each
	// choice's assistant message is one entry in the transformed array.
	var outputTransforms *transform.ChainResult
	if len(source.OutputTransformers) > 0 && len(chatResp.Choices) > 0 {
		msgs := make([]llm.Message, len(chatResp.Choices))
		for i, c := range chatResp.Choices {
			msgs[i] = c.Message
		}
		tCtx := guard.WithCallMeta(ctx, guard.CallMeta{Phase: "output", SourceRoute: source.Route})
		transformed, chain, err := transform.ApplyAll(tCtx, source.OutputTransformers, msgs)
		outputTransforms = chain
		if err != nil {
			return nil, fmt.Errorf("running output transformers: %w", err)
		}
		if len(transformed) != len(chatResp.Choices) {
			return nil, fmt.Errorf("output transformers changed message count: %d != %d", len(transformed), len(chatResp.Choices))
		}
		for i := range chatResp.Choices {
			chatResp.Choices[i].Message = transformed[i]
		}
		log.Info("output transformers completed",
			"transformer_count", len(source.OutputTransformers),
			"changed", chain.Changed,
			"latency_ms", chain.TotalDurationMs,
		)
	}

	// Output guards
	if len(source.OutputGuards) > 0 {
		guardStart := time.Now()
		outputText := llm.ExtractOutputText(chatResp)
		outputCtx := guard.WithCallMeta(ctx, guard.CallMeta{Phase: "output", SourceRoute: source.Route})
		outputResult, err := p.engine.Run(outputCtx, source.OutputGuards, outputText, outputEval)
		metrics.GuardDuration.WithLabelValues("output").Observe(time.Since(guardStart).Seconds())
		if err != nil {
			return nil, fmt.Errorf("running output guards: %w", err)
		}
		log.Info("output guards completed",
			"guard_count", len(source.OutputGuards),
			"passed", outputResult.Passed,
			"latency_ms", time.Since(guardStart).Milliseconds(),
		)
		guardResults["output"] = outputResult

		// Reject at output: LLM was called, but response blocked
		if !outputResult.Passed {
			result = &Result{
				Status:           "rejected",
				Phase:            "output",
				GuardResults:     guardResults,
				Transforms:       transforms,
				OutputTransforms: outputTransforms,
				LatencyMs:        time.Since(start).Milliseconds(),
			}
			log.Info("request rejected at output phase", "total_latency_ms", result.LatencyMs)
			p.record(source, orgID, inputText, guardResults, transforms, outputTransforms, "fail", result.LatencyMs, &chatResp.Usage, chatResp.Model)
			p.alert(ctx, orgID, source.Route, "output", outputResult)
			return result, nil
		}
	}

	result = &Result{
		Status:           "pass",
		LLMResponse:      chatResp,
		GuardResults:     guardResults,
		Transforms:       transforms,
		OutputTransforms: outputTransforms,
		LatencyMs:        time.Since(start).Milliseconds(),
	}
	log.Info("proxy request completed", "status", "pass", "total_latency_ms", result.LatencyMs)
	p.record(source, orgID, inputText, guardResults, transforms, outputTransforms, "pass", result.LatencyMs, &chatResp.Usage, chatResp.Model)
	return result, nil
}

func (p *Processor) record(source *backendconfig.ResolvedSource, orgID uuid.UUID, inputText string, guardResults map[string]*guard.EngineResult, transforms, outputTransforms *transform.ChainResult, overallResult string, latencyMs int64, usage *llm.Usage, model string) {
	if p.audit != nil {
		p.audit.Record(orgID, source.ID, inputText, guardResults, transforms, outputTransforms, overallResult, latencyMs, usage, model)
	}
}
