package processor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/shared/middleware"
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

	// LatencyMs is the total wall-clock time for the request.
	LatencyMs int64 `json:"latency_ms"`
}

// AuditSink receives completed proxy results for asynchronous recording.
type AuditSink interface {
	Record(orgID, sourceID uuid.UUID, inputText string, guardResults map[string]*guard.EngineResult, overallResult string, latencyMs int64)
}

// Processor runs the proxy stages: input guards → LLM call → output guards.
// All configuration comes from the in-memory store; no I/O beyond the LLM
// call happens on the request path.
type Processor struct {
	store     *backendconfig.Store
	engine    *guard.Engine
	llmClient *llm.Client
	audit     AuditSink
}

// NewProcessor creates a processor. audit may be nil to disable audit logging.
func NewProcessor(store *backendconfig.Store, engine *guard.Engine, llmClient *llm.Client, audit AuditSink) *Processor {
	return &Processor{store: store, engine: engine, llmClient: llmClient, audit: audit}
}

// requestID extracts the request ID from the context.
func requestID(ctx context.Context) string {
	if id, ok := ctx.Value(middleware.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// Execute runs the full proxy flow for a given source and chat request.
func (p *Processor) Execute(ctx context.Context, orgID uuid.UUID, sourceRoute string, chatReq *llm.ChatRequest) (*Result, error) {
	start := time.Now()
	log := slog.With("request_id", requestID(ctx), "source", sourceRoute)
	log.Info("proxy request started")

	source, ok := p.store.LookupSource(orgID, sourceRoute)
	if !ok {
		return nil, fmt.Errorf("source not found: %q", sourceRoute)
	}
	if !source.Enabled {
		return nil, fmt.Errorf("source %q is disabled", sourceRoute)
	}

	evalCfg := guard.EvalConfig{
		Criteria: guard.PassCriteria(source.PassCriteria),
	}
	if source.PassThreshold != nil {
		evalCfg.Threshold = *source.PassThreshold
	}

	inputText := llm.ExtractInputText(chatReq)

	// Input guards
	guardResults := make(map[string]*guard.EngineResult)
	if len(source.InputGuards) > 0 {
		guardStart := time.Now()
		inputCtx := guard.WithCallMeta(ctx, guard.CallMeta{Phase: "input", SourceRoute: source.Route})
		inputResult, err := p.engine.Run(inputCtx, source.InputGuards, inputText, evalCfg)
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
			result := &Result{
				Status:       "rejected",
				Phase:        "input",
				GuardResults: guardResults,
				LatencyMs:    time.Since(start).Milliseconds(),
			}
			log.Info("request rejected at input phase", "total_latency_ms", result.LatencyMs)
			p.record(source, orgID, inputText, guardResults, "fail", result.LatencyMs)
			return result, nil
		}
	}

	// Call LLM via gateway, overriding the model with the source's config
	chatReq.Model = source.LLMModel

	llmStart := time.Now()
	log.Info("calling LLM gateway", "model", chatReq.Model)

	chatResp, err := p.llmClient.ChatCompletion(ctx, source.LLMAPIKey, chatReq)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}
	log.Info("LLM call completed", "latency_ms", time.Since(llmStart).Milliseconds())

	// Output guards
	if len(source.OutputGuards) > 0 {
		guardStart := time.Now()
		outputText := llm.ExtractOutputText(chatResp)
		outputCtx := guard.WithCallMeta(ctx, guard.CallMeta{Phase: "output", SourceRoute: source.Route})
		outputResult, err := p.engine.Run(outputCtx, source.OutputGuards, outputText, evalCfg)
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
			result := &Result{
				Status:       "rejected",
				Phase:        "output",
				GuardResults: guardResults,
				LatencyMs:    time.Since(start).Milliseconds(),
			}
			log.Info("request rejected at output phase", "total_latency_ms", result.LatencyMs)
			p.record(source, orgID, inputText, guardResults, "fail", result.LatencyMs)
			return result, nil
		}
	}

	result := &Result{
		Status:       "pass",
		LLMResponse:  chatResp,
		GuardResults: guardResults,
		LatencyMs:    time.Since(start).Milliseconds(),
	}
	log.Info("proxy request completed", "status", "pass", "total_latency_ms", result.LatencyMs)
	p.record(source, orgID, inputText, guardResults, "pass", result.LatencyMs)
	return result, nil
}

func (p *Processor) record(source *backendconfig.ResolvedSource, orgID uuid.UUID, inputText string, guardResults map[string]*guard.EngineResult, overallResult string, latencyMs int64) {
	if p.audit != nil {
		p.audit.Record(orgID, source.ID, inputText, guardResults, overallResult, latencyMs)
	}
}
