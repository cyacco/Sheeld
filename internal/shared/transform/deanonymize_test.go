package transform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

// fakeAnalyzer flags every occurrence of the given needles with the given
// entity type.
func fakeAnalyzer(t *testing.T, entityType string, needles ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("analyze decode: %v", err)
		}
		results := []presidioAnalyzerResult{}
		for _, needle := range needles {
			for idx := 0; ; {
				i := strings.Index(req.Text[idx:], needle)
				if i < 0 {
					break
				}
				start := idx + i
				results = append(results, presidioAnalyzerResult{
					EntityType: entityType, Start: start, End: start + len(needle), Score: 0.9,
				})
				idx = start + len(needle)
			}
		}
		json.NewEncoder(w).Encode(results)
	}))
}

func TestReversibleAnonymizeAndDeanonymize(t *testing.T) {
	srv := fakeAnalyzer(t, "PERSON", "Alice", "Bob")
	defer srv.Close()

	anon := NewPresidioTransformer("anon", PresidioConfig{
		AnalyzerURL: srv.URL,
		Mode:        "reversible",
	})
	dean := NewDeanonymizeTransformer("dean")

	ctx := WithState(context.Background())

	in := []llm.Message{
		{Role: "user", Content: "Alice emailed Bob"},
		{Role: "user", Content: "did Alice reply?"},
	}
	anonymized, err := anon.Transform(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	// Both names replaced; same original → same placeholder across messages.
	if strings.Contains(anonymized[0].Content, "Alice") || strings.Contains(anonymized[0].Content, "Bob") {
		t.Errorf("PII not anonymized: %q", anonymized[0].Content)
	}
	alicePH := strings.TrimSuffix(strings.TrimSpace(strings.Split(anonymized[1].Content, " ")[1]), "?")
	if !strings.HasPrefix(alicePH, "<PERSON_") {
		t.Fatalf("unexpected placeholder in %q", anonymized[1].Content)
	}
	if !strings.HasPrefix(anonymized[0].Content, alicePH+" emailed") {
		t.Errorf("same original should reuse placeholder: %q vs %q", anonymized[0].Content, anonymized[1].Content)
	}
	if in[0].Content != "Alice emailed Bob" {
		t.Error("input slice was mutated")
	}

	// Simulate an LLM response that echoes the placeholders.
	llmOut := []llm.Message{{Role: "assistant", Content: "Yes, " + alicePH + " replied to the thread."}}
	restored, err := dean.Transform(ctx, llmOut)
	if err != nil {
		t.Fatal(err)
	}
	if restored[0].Content != "Yes, Alice replied to the thread." {
		t.Errorf("deanonymize failed: %q", restored[0].Content)
	}
}

func TestReversibleRequiresState(t *testing.T) {
	srv := fakeAnalyzer(t, "PERSON", "Alice")
	defer srv.Close()
	anon := NewPresidioTransformer("anon", PresidioConfig{AnalyzerURL: srv.URL, Mode: "reversible"})
	if _, err := anon.Transform(context.Background(), []llm.Message{{Role: "user", Content: "Alice"}}); err == nil {
		t.Error("expected error without request state")
	}
}

func TestDeanonymizeWithoutStateIsNoop(t *testing.T) {
	dean := NewDeanonymizeTransformer("dean")
	in := []llm.Message{{Role: "assistant", Content: "hello <PERSON_1>"}}
	out, err := dean.Transform(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Content != "hello <PERSON_1>" {
		t.Errorf("no-op expected: %q", out[0].Content)
	}
}

func TestPresidioModeValidation(t *testing.T) {
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"http://a:3000","mode":"reversible"}`)); err != nil {
		t.Errorf("reversible without anonymizer_url should be valid: %v", err)
	}
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"http://a:3000","mode":"redact"}`)); err == nil {
		t.Error("redact without anonymizer_url should be invalid")
	}
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"http://a:3000","mode":"bogus"}`)); err == nil {
		t.Error("unknown mode should be invalid")
	}
	if _, err := deanonymizeFactory("t", json.RawMessage(`{}`)); err != nil {
		t.Errorf("deanonymize factory: %v", err)
	}
}

func TestStateAllocatePlaceholder(t *testing.T) {
	ctx := WithState(context.Background())
	s, _ := StateFrom(ctx)
	p1 := s.AllocatePlaceholder("PERSON", "Alice")
	p2 := s.AllocatePlaceholder("PERSON", "Bob")
	p3 := s.AllocatePlaceholder("PERSON", "Alice")
	if p1 != "<PERSON_1>" || p2 != "<PERSON_2>" || p3 != p1 {
		t.Errorf("got %s %s %s", p1, p2, p3)
	}
	if s.Len() != 2 {
		t.Errorf("expected 2 mappings, got %d", s.Len())
	}
}
