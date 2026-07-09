package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// roundTrip unmarshals into T and marshals back, returning the output JSON as
// a generic map for field assertions.
func roundTripRequest(t *testing.T, in string) (ChatRequest, map[string]any) {
	t.Helper()
	var req ChatRequest
	if err := json.Unmarshal([]byte(in), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	return req, m
}

func TestChatRequest_PreservesUnknownFields(t *testing.T) {
	in := `{
		"model": "gpt-4o",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [{"type":"function","function":{"name":"get_weather"}}],
		"tool_choice": "auto",
		"response_format": {"type":"json_object"},
		"temperature": 0.7,
		"top_p": 0.9,
		"seed": 42,
		"stop": ["\n"]
	}`
	req, out := roundTripRequest(t, in)

	if req.Model != "gpt-4o" || len(req.Messages) != 1 {
		t.Fatalf("core fields wrong: model=%q messages=%d", req.Model, len(req.Messages))
	}
	// Every non-core field must survive the round trip — dropping tools
	// silently breaks function calling.
	for _, key := range []string{"tools", "tool_choice", "response_format", "temperature", "top_p", "seed", "stop"} {
		if _, ok := out[key]; !ok {
			t.Errorf("field %q was dropped", key)
		}
	}
	if out["tool_choice"] != "auto" {
		t.Errorf("tool_choice value changed: %v", out["tool_choice"])
	}
}

func TestMessage_MultimodalContentPreservedAndTextExtracted(t *testing.T) {
	in := `{"role":"user","content":[
		{"type":"text","text":"describe this"},
		{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}
	]}`
	var msg Message
	if err := json.Unmarshal([]byte(in), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Guards see the text parts.
	if msg.Content != "describe this" {
		t.Fatalf("extracted text = %q, want %q", msg.Content, "describe this")
	}
	// Untouched, the array (including the image) re-emits verbatim.
	out, _ := json.Marshal(msg)
	if !strings.Contains(string(out), "image_url") || !strings.Contains(string(out), "x.png") {
		t.Fatalf("multimodal content not preserved: %s", out)
	}
	if !strings.Contains(string(out), `"content":[`) {
		t.Fatalf("content should re-emit as an array: %s", out)
	}
}

func TestMessage_RewrittenMultimodalBecomesString(t *testing.T) {
	in := `{"role":"user","content":[{"type":"text","text":"secret"}]}`
	var msg Message
	if err := json.Unmarshal([]byte(in), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// A transformer rewrites the text.
	msg.Content = "[redacted]"
	out, _ := json.Marshal(msg)
	if !strings.Contains(string(out), `"content":"[redacted]"`) {
		t.Fatalf("rewritten content should emit as string: %s", out)
	}
}

func TestMessage_NullContentPreserved(t *testing.T) {
	// Assistant tool-call messages carry content:null and tool_calls.
	in := `{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function"}]}`
	var msg Message
	if err := json.Unmarshal([]byte(in), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, _ := json.Marshal(msg)
	if !strings.Contains(string(out), `"content":null`) {
		t.Errorf("null content should re-emit as null: %s", out)
	}
	if !strings.Contains(string(out), "tool_calls") || !strings.Contains(string(out), "call_1") {
		t.Errorf("tool_calls dropped: %s", out)
	}
}

func TestChatResponse_PreservesToolCallsAndUnknownFields(t *testing.T) {
	in := `{
		"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o",
		"system_fingerprint":"fp_abc",
		"choices":[{"index":0,"finish_reason":"tool_calls","logprobs":null,
			"message":{"role":"assistant","content":null,
				"tool_calls":[{"id":"call_9","type":"function","function":{"name":"lookup"}}]}}],
		"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}
	}`
	var resp ChatResponse
	if err := json.Unmarshal([]byte(in), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "chatcmpl-1" || resp.Usage.TotalTokens != 3 {
		t.Fatalf("core fields wrong: %+v", resp)
	}
	out, _ := json.Marshal(resp)
	s := string(out)
	for _, want := range []string{"system_fingerprint", "fp_abc", "tool_calls", "call_9", `"finish_reason":"tool_calls"`} {
		if !strings.Contains(s, want) {
			t.Errorf("response dropped %q: %s", want, s)
		}
	}
}
