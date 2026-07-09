package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

// fakePresidio serves /analyze and /anonymize: it flags every occurrence of
// "Alice" as PERSON and replaces flagged spans with <ENTITY_TYPE>.
func fakePresidio(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text     string `json:"text"`
			Language string `json:"language"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("analyze decode: %v", err)
		}
		if req.Language == "" {
			t.Error("analyze request missing language")
		}
		results := []presidioAnalyzerResult{}
		for idx := 0; ; {
			i := strings.Index(req.Text[idx:], "Alice")
			if i < 0 {
				break
			}
			start := idx + i
			results = append(results, presidioAnalyzerResult{
				EntityType: "PERSON", Start: start, End: start + len("Alice"), Score: 0.85,
			})
			idx = start + len("Alice")
		}
		json.NewEncoder(w).Encode(results)
	})
	mux.HandleFunc("/anonymize", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text            string                   `json:"text"`
			AnalyzerResults []presidioAnalyzerResult `json:"analyzer_results"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("anonymize decode: %v", err)
		}
		out := req.Text
		for i := len(req.AnalyzerResults) - 1; i >= 0; i-- {
			res := req.AnalyzerResults[i]
			out = out[:res.Start] + fmt.Sprintf("<%s>", res.EntityType) + out[res.End:]
		}
		json.NewEncoder(w).Encode(map[string]string{"text": out})
	})
	return httptest.NewServer(mux)
}

func TestPresidioTransform(t *testing.T) {
	srv := fakePresidio(t)
	defer srv.Close()

	tr := NewPresidioTransformer("pii", PresidioConfig{
		AnalyzerURL:   srv.URL,
		AnonymizerURL: srv.URL,
	})

	in := []llm.Message{
		{Role: "user", Content: "Alice called Alice back"},
		{Role: "user", Content: "nothing sensitive here"},
		{Role: "user", Content: ""},
	}
	out, err := tr.Transform(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out[0].Content, "<PERSON> called <PERSON> back"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if out[1].Content != "nothing sensitive here" {
		t.Errorf("clean message changed: %q", out[1].Content)
	}
	if in[0].Content != "Alice called Alice back" {
		t.Error("input slice was mutated")
	}
}

func TestPresidioAnalyzerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	tr := NewPresidioTransformer("pii", PresidioConfig{AnalyzerURL: srv.URL, AnonymizerURL: srv.URL})
	if _, err := tr.Transform(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestPresidioFactoryValidation(t *testing.T) {
	if _, err := presidioFactory("t", json.RawMessage(`{"anonymizer_url":"http://x"}`)); err == nil {
		t.Error("missing analyzer_url accepted")
	}
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"http://x"}`)); err == nil {
		t.Error("missing anonymizer_url accepted")
	}
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"http://a:3000","anonymizer_url":"http://b:3000"}`)); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}
