package guard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPresidioGuardDetection(t *testing.T) {
	var gotReq map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/analyze") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode: %v", err)
		}
		if strings.Contains(gotReq["text"].(string), "4111") {
			w.Write([]byte(`[{"entity_type":"CREDIT_CARD","start":11,"end":27,"score":0.9}]`))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	g := NewPresidioGuard("pii", PresidioConfig{
		AnalyzerURL: srv.URL,
		Entities:    []string{"CREDIT_CARD"},
	})

	res, err := g.Validate(context.Background(), "my card is 4111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed || !strings.Contains(res.Message, "CREDIT_CARD") {
		t.Errorf("expected CREDIT_CARD rejection, got passed=%v msg=%q", res.Passed, res.Message)
	}
	if gotReq["language"] != "en" || gotReq["score_threshold"] != 0.5 {
		t.Errorf("defaults not applied: %v", gotReq)
	}
	if ents, _ := gotReq["entities"].([]interface{}); len(ents) != 1 || ents[0] != "CREDIT_CARD" {
		t.Errorf("entities not forwarded: %v", gotReq["entities"])
	}

	res, err = g.Validate(context.Background(), "nothing sensitive")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed || res.Message != "no PII detected" {
		t.Errorf("clean text should pass: passed=%v msg=%q", res.Passed, res.Message)
	}
}

func TestPresidioGuardErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	g := NewPresidioGuard("pii", PresidioConfig{AnalyzerURL: srv.URL})
	if _, err := g.Validate(context.Background(), "hi"); err == nil {
		t.Error("expected error on 500, got nil")
	}
}

func TestPresidioGuardFactoryValidation(t *testing.T) {
	if _, err := presidioFactory("t", json.RawMessage(`{}`)); err == nil {
		t.Error("missing analyzer_url accepted")
	}
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"ftp://x"}`)); err == nil {
		t.Error("non-http url accepted")
	}
	if _, err := presidioFactory("t", json.RawMessage(`{"analyzer_url":"http://analyzer:3000"}`)); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}
