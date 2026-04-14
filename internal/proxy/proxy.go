package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sheeld/sheeld/internal/api/middleware"
	"github.com/sheeld/sheeld/internal/crypto"
	"github.com/sheeld/sheeld/internal/db/generated"
	"github.com/sheeld/sheeld/internal/guard"
	"github.com/sheeld/sheeld/internal/llm"
)

// ProxyResult is the full result of a proxy request.
type ProxyResult struct {
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

// Proxy orchestrates the full Sheeld flow:
// input guards → LLM call → output guards → response.
type Proxy struct {
	queries       *generated.Queries
	engine        *guard.Engine
	llmClient     *llm.Client
	encryptionKey string
}

// NewProxy creates a new proxy orchestrator.
func NewProxy(queries *generated.Queries, engine *guard.Engine, llmClient *llm.Client, encryptionKey string) *Proxy {
	return &Proxy{
		queries:       queries,
		engine:        engine,
		llmClient:     llmClient,
		encryptionKey: encryptionKey,
	}
}

// requestID extracts the request ID from the context.
func requestID(ctx context.Context) string {
	if id, ok := ctx.Value(middleware.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// Execute runs the full proxy flow for a given source and chat request.
func (p *Proxy) Execute(ctx context.Context, orgID uuid.UUID, sourceRoute string, chatReq *llm.ChatRequest) (*ProxyResult, error) {
	start := time.Now()
	reqID := requestID(ctx)

	log := slog.With("request_id", reqID, "source", sourceRoute)
	log.Info("proxy request started")

	// 1. Look up source by route + org
	source, err := p.queries.GetSourceByRoute(ctx, generated.GetSourceByRouteParams{
		Route:          sourceRoute,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("source not found: %w", err)
	}

	if !source.Enabled {
		return nil, fmt.Errorf("source %q is disabled", sourceRoute)
	}

	// 2. Load enabled guardrails
	guardrails, err := p.queries.ListEnabledGuardrailsBySource(ctx, source.ID)
	if err != nil {
		return nil, fmt.Errorf("loading guardrails: %w", err)
	}

	// 3. Separate into input and output guards
	inputGuards, err := p.buildGuards(guardrails, "input")
	if err != nil {
		return nil, fmt.Errorf("building input guards: %w", err)
	}
	outputGuards, err := p.buildGuards(guardrails, "output")
	if err != nil {
		return nil, fmt.Errorf("building output guards: %w", err)
	}

	// Build eval config from source settings
	evalCfg := guard.EvalConfig{
		Criteria: guard.PassCriteria(source.PassCriteria),
	}
	if source.PassThreshold.Valid {
		evalCfg.Threshold = int(source.PassThreshold.Int32)
	}

	// 4. Extract input text (last user message)
	inputText := llm.ExtractInputText(chatReq)

	// 5. Run input guards
	guardResults := make(map[string]*guard.EngineResult)
	if len(inputGuards) > 0 {
		guardStart := time.Now()
		inputResult, err := p.engine.Run(ctx, inputGuards, inputText, evalCfg)
		if err != nil {
			return nil, fmt.Errorf("running input guards: %w", err)
		}
		log.Info("input guards completed",
			"guard_count", len(inputGuards),
			"passed", inputResult.Passed,
			"latency_ms", time.Since(guardStart).Milliseconds(),
		)
		guardResults["input"] = inputResult

		// 6. If input fails → reject (no LLM call, tokens saved)
		if !inputResult.Passed {
			result := &ProxyResult{
				Status:       "rejected",
				Phase:        "input",
				GuardResults: guardResults,
				LatencyMs:    time.Since(start).Milliseconds(),
			}
			log.Info("request rejected at input phase", "total_latency_ms", result.LatencyMs)
			p.writeAuditLog(ctx, source, orgID, inputText, guardResults, "fail", result.LatencyMs)
			return result, nil
		}
	}

	// 7. Call LLM via gateway
	// Shallow-copy the request before overriding the model so we don't
	// mutate the caller-supplied struct. The Messages slice is shared
	// (read-only from here on), which is fine.
	reqCopy := *chatReq
	reqCopy.Model = source.LlmModel

	apiKey, err := crypto.Decrypt(source.LlmApiKeyEnc, p.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting API key: %w", err)
	}

	llmStart := time.Now()
	log.Info("calling LLM gateway", "model", reqCopy.Model)

	chatResp, err := p.llmClient.ChatCompletion(ctx, apiKey, &reqCopy)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}
	log.Info("LLM call completed", "latency_ms", time.Since(llmStart).Milliseconds())

	// 8. Run output guards
	if len(outputGuards) > 0 {
		guardStart := time.Now()
		outputText := llm.ExtractOutputText(chatResp)
		outputResult, err := p.engine.Run(ctx, outputGuards, outputText, evalCfg)
		if err != nil {
			return nil, fmt.Errorf("running output guards: %w", err)
		}
		log.Info("output guards completed",
			"guard_count", len(outputGuards),
			"passed", outputResult.Passed,
			"latency_ms", time.Since(guardStart).Milliseconds(),
		)
		guardResults["output"] = outputResult

		// 9. If output fails → reject (LLM was called, but response blocked)
		if !outputResult.Passed {
			result := &ProxyResult{
				Status:       "rejected",
				Phase:        "output",
				GuardResults: guardResults,
				LatencyMs:    time.Since(start).Milliseconds(),
			}
			log.Info("request rejected at output phase", "total_latency_ms", result.LatencyMs)
			p.writeAuditLog(ctx, source, orgID, inputText, guardResults, "fail", result.LatencyMs)
			return result, nil
		}
	}

	// 10. Everything passed — return the LLM response
	result := &ProxyResult{
		Status:       "pass",
		LLMResponse:  chatResp,
		GuardResults: guardResults,
		LatencyMs:    time.Since(start).Milliseconds(),
	}
	log.Info("proxy request completed", "status", "pass", "total_latency_ms", result.LatencyMs)
	p.writeAuditLog(ctx, source, orgID, inputText, guardResults, "pass", result.LatencyMs)
	return result, nil
}

// buildGuards creates Guard instances for guardrails matching the given phase.
func (p *Proxy) buildGuards(guardrails []generated.Guardrail, phase string) ([]guard.Guard, error) {
	var guards []guard.Guard
	for _, gr := range guardrails {
		if gr.Phase != phase && gr.Phase != "both" {
			continue
		}
		g, err := p.engine.Registry().Create(gr.GuardType, gr.Name, gr.Config)
		if err != nil {
			return nil, fmt.Errorf("creating guard %q (type %s): %w", gr.Name, gr.GuardType, err)
		}
		guards = append(guards, g)
	}
	return guards, nil
}

// writeAuditLog records the proxy result asynchronously.
func (p *Proxy) writeAuditLog(
	ctx context.Context,
	source generated.Source,
	orgID uuid.UUID,
	inputText string,
	guardResults map[string]*guard.EngineResult,
	overallResult string,
	latencyMs int64,
) {
	// Hash the input for tracking without storing raw content
	hash := sha256.Sum256([]byte(inputText))
	inputHash := hex.EncodeToString(hash[:])

	guardResultsJSON, err := json.Marshal(guardResults)
	if err != nil {
		slog.Error("failed to marshal guard results for audit log", "error", err)
		return
	}

	_, err = p.queries.CreateAuditLog(ctx, generated.CreateAuditLogParams{
		OrganizationID: orgID,
		SourceID:       source.ID,
		InputHash:      pgtype.Text{String: inputHash, Valid: true},
		GuardResults:   guardResultsJSON,
		OverallResult:  overallResult,
		LatencyMs:      int32(latencyMs),
	})
	if err != nil {
		slog.Error("failed to write audit log", "error", err, "source", source.Route)
	}
}
