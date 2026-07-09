package transform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

// PresidioConfig holds configuration for the presidio transformer, which
// redacts PII via self-hosted Microsoft Presidio analyzer + anonymizer
// services.
type PresidioConfig struct {
	// AnalyzerURL is the base URL of the presidio-analyzer service
	// (e.g. http://presidio-analyzer:3000). /analyze is appended.
	AnalyzerURL string `json:"analyzer_url"`

	// AnonymizerURL is the base URL of the presidio-anonymizer service
	// (e.g. http://presidio-anonymizer:3000). /anonymize is appended.
	// Required in "redact" mode; unused in "reversible" mode.
	AnonymizerURL string `json:"anonymizer_url,omitempty"`

	// Mode is "redact" (default — detected entities become <ENTITY_TYPE>
	// placeholders via the anonymizer, irreversibly) or "reversible" —
	// entities become numbered placeholders (<PERSON_1>) substituted
	// locally, with the originals recorded in the request State so a
	// deanonymize transformer on the output chain can restore them.
	Mode string `json:"mode,omitempty"`

	// Language is the ISO 639-1 analysis language. Defaults to "en".
	Language string `json:"language,omitempty"`

	// Entities restricts detection to these entity types (e.g.
	// ["PERSON","EMAIL_ADDRESS"]). Empty means all supported entities.
	Entities []string `json:"entities,omitempty"`

	// ScoreThreshold drops detections below this confidence. Zero means
	// the analyzer default.
	ScoreThreshold float64 `json:"score_threshold,omitempty"`

	// TimeoutSeconds is the HTTP timeout per call. Defaults to 10.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// PresidioTransformer redacts PII from every message's content: each message
// is analyzed (POST /analyze) and, when entities are found, anonymized
// (POST /anonymize) with Presidio's default replace operator, which
// substitutes <ENTITY_TYPE> placeholders.
type PresidioTransformer struct {
	name   string
	cfg    PresidioConfig
	client *http.Client
}

// NewPresidioTransformer creates a new PresidioTransformer from configuration.
func NewPresidioTransformer(name string, cfg PresidioConfig) *PresidioTransformer {
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = webhookDefaultTimeout
	}
	return &PresidioTransformer{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (t *PresidioTransformer) Name() string { return t.name }
func (t *PresidioTransformer) Type() string { return "presidio" }

// presidioAnalyzerResult is one detection from /analyze, passed through
// verbatim to /anonymize.
type presidioAnalyzerResult struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

func (t *PresidioTransformer) Transform(ctx context.Context, msgs []llm.Message) ([]llm.Message, error) {
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		if out[i].Content == "" {
			continue
		}
		var redacted string
		var err error
		if t.cfg.Mode == "reversible" {
			redacted, err = t.anonymizeReversibly(ctx, out[i].Content)
		} else {
			redacted, err = t.redact(ctx, out[i].Content)
		}
		if err != nil {
			return nil, err
		}
		out[i].Content = redacted
	}
	return out, nil
}

func (t *PresidioTransformer) analyze(ctx context.Context, text string) ([]presidioAnalyzerResult, error) {
	analyzeReq := map[string]interface{}{
		"text":     text,
		"language": t.cfg.Language,
	}
	if len(t.cfg.Entities) > 0 {
		analyzeReq["entities"] = t.cfg.Entities
	}
	if t.cfg.ScoreThreshold > 0 {
		analyzeReq["score_threshold"] = t.cfg.ScoreThreshold
	}
	var results []presidioAnalyzerResult
	if err := t.post(ctx, t.cfg.AnalyzerURL+"/analyze", analyzeReq, &results); err != nil {
		return nil, fmt.Errorf("presidio analyze: %w", err)
	}
	return results, nil
}

func (t *PresidioTransformer) redact(ctx context.Context, text string) (string, error) {
	results, err := t.analyze(ctx, text)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return text, nil
	}

	anonymizeReq := map[string]interface{}{
		"text":             text,
		"analyzer_results": results,
	}
	var anon struct {
		Text string `json:"text"`
	}
	if err := t.post(ctx, t.cfg.AnonymizerURL+"/anonymize", anonymizeReq, &anon); err != nil {
		return "", fmt.Errorf("presidio anonymize: %w", err)
	}
	return anon.Text, nil
}

// anonymizeReversibly substitutes numbered placeholders locally from the
// analyzer's spans and records placeholder → original mappings in the
// request State. The same original value maps to the same placeholder so
// the LLM sees a coherent conversation.
func (t *PresidioTransformer) anonymizeReversibly(ctx context.Context, text string) (string, error) {
	results, err := t.analyze(ctx, text)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return text, nil
	}
	state, ok := StateFrom(ctx)
	if !ok {
		return "", fmt.Errorf("presidio: reversible mode requires request state (are you running outside the proxy pipeline?)")
	}

	// Replace back to front so earlier spans stay valid.
	sort.Slice(results, func(i, j int) bool { return results[i].Start > results[j].Start })
	out := text
	for _, r := range results {
		if r.Start < 0 || r.End > len(out) || r.Start >= r.End {
			continue
		}
		out = out[:r.Start] + state.AllocatePlaceholder(r.EntityType, out[r.Start:r.End]) + out[r.End:]
	}
	return out, nil
}

func (t *PresidioTransformer) post(ctx context.Context, url string, reqBody interface{}, respBody interface{}) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, webhookMaxResponseBytes)).Decode(respBody); err != nil {
		return fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return nil
}
