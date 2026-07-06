package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const presidioDefaultTimeout = 10

// PresidioConfig holds configuration for the presidio guard, which rejects
// text containing PII detected by a self-hosted Microsoft Presidio analyzer.
// (The presidio transformer redacts instead; this guard blocks.)
type PresidioConfig struct {
	// AnalyzerURL is the base URL of the presidio-analyzer service
	// (e.g. http://presidio-analyzer:3000). /analyze is appended.
	AnalyzerURL string `json:"analyzer_url"`

	// Language is the ISO 639-1 analysis language. Defaults to "en".
	Language string `json:"language,omitempty"`

	// Entities restricts detection to these entity types (e.g.
	// ["CREDIT_CARD","US_SSN"]). Empty means all supported entities.
	Entities []string `json:"entities,omitempty"`

	// ScoreThreshold drops detections below this confidence (0.0–1.0).
	// Defaults to 0.5.
	ScoreThreshold float64 `json:"score_threshold,omitempty"`

	// TimeoutSeconds is the HTTP timeout. Defaults to 10.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// PresidioGuard fails validation when the analyzer detects any matching
// entity at or above the score threshold. Connection errors and non-200
// responses are errors, so the on_error policy applies.
type PresidioGuard struct {
	name   string
	cfg    PresidioConfig
	client *http.Client
}

// NewPresidioGuard creates a new PresidioGuard from configuration.
func NewPresidioGuard(name string, cfg PresidioConfig) *PresidioGuard {
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	if cfg.ScoreThreshold <= 0 {
		cfg.ScoreThreshold = 0.5
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = presidioDefaultTimeout
	}
	return &PresidioGuard{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (g *PresidioGuard) Type() string { return "presidio" }
func (g *PresidioGuard) Name() string { return g.name }

// presidioDetection is one analyzer result; positions are recorded in the
// audit details, never the matched text itself.
type presidioDetection struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

func (g *PresidioGuard) Validate(ctx context.Context, input string) (*Result, error) {
	start := time.Now()

	analyzeReq := map[string]interface{}{
		"text":            input,
		"language":        g.cfg.Language,
		"score_threshold": g.cfg.ScoreThreshold,
	}
	if len(g.cfg.Entities) > 0 {
		analyzeReq["entities"] = g.cfg.Entities
	}
	body, err := json.Marshal(analyzeReq)
	if err != nil {
		return nil, fmt.Errorf("presidio: marshal request: %w", err)
	}

	url := strings.TrimRight(g.cfg.AnalyzerURL, "/") + "/analyze"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("presidio: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("presidio: analyzer call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("presidio: analyzer returned status %d", resp.StatusCode)
	}

	var detections []presidioDetection
	if err := json.NewDecoder(resp.Body).Decode(&detections); err != nil {
		return nil, fmt.Errorf("presidio: decode response: %w", err)
	}

	duration := time.Since(start)

	if len(detections) > 0 {
		types := make([]string, 0, len(detections))
		seen := make(map[string]bool)
		for _, d := range detections {
			if !seen[d.EntityType] {
				seen[d.EntityType] = true
				types = append(types, d.EntityType)
			}
		}
		return &Result{
			GuardName: g.name,
			GuardType: g.Type(),
			Passed:    false,
			Message:   "detected PII: " + strings.Join(types, ", "),
			Details:   map[string]interface{}{"detections": detections, "threshold": g.cfg.ScoreThreshold},
			Duration:  duration,
		}, nil
	}

	return &Result{
		GuardName: g.name,
		GuardType: g.Type(),
		Passed:    true,
		Message:   "no PII detected",
		Duration:  duration,
	}, nil
}
