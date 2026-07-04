package guard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockOpenAIModServer returns a test server that responds with the given category scores.
func mockOpenAIModServer(t *testing.T, scores map[string]float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/moderations" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		cats := make(map[string]bool, len(scores))
		for k, v := range scores {
			cats[k] = v >= 0.5
		}
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"flagged":         false,
					"categories":      cats,
					"category_scores": scores,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestOpenAIModerationGuard_Pass(t *testing.T) {
	srv := mockOpenAIModServer(t, map[string]float64{
		"hate":     0.01,
		"violence": 0.02,
		"sexual":   0.00,
	})
	defer srv.Close()

	g := NewOpenAIModerationGuard("mod", OpenAIModerationConfig{
		APIKey:     "test-key",
		Categories: []string{"hate", "violence"},
		Threshold:  0.7,
		BaseURL:    srv.URL,
	})

	result, err := g.Validate(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got fail: %s", result.Message)
	}
}

func TestOpenAIModerationGuard_Fail_ThresholdExceeded(t *testing.T) {
	srv := mockOpenAIModServer(t, map[string]float64{
		"hate":     0.85,
		"violence": 0.10,
	})
	defer srv.Close()

	g := NewOpenAIModerationGuard("mod", OpenAIModerationConfig{
		APIKey:     "test-key",
		Categories: []string{"hate", "violence"},
		Threshold:  0.7,
		BaseURL:    srv.URL,
	})

	result, err := g.Validate(context.Background(), "some hateful content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected fail, got pass")
	}
	if _, ok := result.Details["exceeded_categories"]; !ok {
		t.Error("expected exceeded_categories in details")
	}
}

func TestOpenAIModerationGuard_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := NewOpenAIModerationGuard("mod", OpenAIModerationConfig{
		APIKey:    "test-key",
		Threshold: 0.7,
		BaseURL:   srv.URL,
	})

	_, err := g.Validate(context.Background(), "test")
	if err == nil {
		t.Error("expected error from API failure, got nil")
	}
}

func TestOpenAIModerationGuard_AllCategoriesCheckedWhenNoneConfigured(t *testing.T) {
	srv := mockOpenAIModServer(t, map[string]float64{
		"hate":     0.01,
		"violence": 0.95, // over threshold
	})
	defer srv.Close()

	// No categories configured — should check all returned by the API.
	g := NewOpenAIModerationGuard("mod", OpenAIModerationConfig{
		APIKey:    "test-key",
		Threshold: 0.7,
		BaseURL:   srv.URL,
	})

	result, err := g.Validate(context.Background(), "violent content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected fail when unconfigured category exceeds threshold")
	}
}

func TestOpenAIModerationGuard_DefaultThreshold(t *testing.T) {
	srv := mockOpenAIModServer(t, map[string]float64{
		"hate": 0.6, // over default threshold of 0.5
	})
	defer srv.Close()

	// Threshold deliberately omitted — should default to 0.5.
	g := NewOpenAIModerationGuard("mod", OpenAIModerationConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	result, err := g.Validate(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected fail with default threshold 0.5 and score 0.6")
	}
}
