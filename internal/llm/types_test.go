package llm

import (
	"encoding/json"
	"testing"
)

func TestExtractInputText(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     string
	}{
		{
			name: "single user message",
			messages: []Message{
				{Role: "user", Content: StringContent("Hello")},
			},
			want: "Hello",
		},
		{
			name: "system + user",
			messages: []Message{
				{Role: "system", Content: StringContent("You are helpful")},
				{Role: "user", Content: StringContent("What is 2+2?")},
			},
			want: "What is 2+2?",
		},
		{
			name: "multi-turn takes last user message",
			messages: []Message{
				{Role: "user", Content: StringContent("First question")},
				{Role: "assistant", Content: StringContent("First answer")},
				{Role: "user", Content: StringContent("Follow-up question")},
			},
			want: "Follow-up question",
		},
		{
			name:     "no user messages",
			messages: []Message{{Role: "system", Content: StringContent("You are helpful")}},
			want:     "",
		},
		{
			name:     "empty messages",
			messages: []Message{},
			want:     "",
		},
		{
			name: "multi-part text + image content",
			messages: []Message{
				{
					Role: "user",
					Content: json.RawMessage(`[
						{"type": "text", "text": "What is in this image?"},
						{"type": "image_url", "image_url": {"url": "https://example.com/cat.jpg"}}
					]`),
				},
			},
			want: "What is in this image?",
		},
		{
			name: "multi-part multiple text segments concatenate",
			messages: []Message{
				{
					Role: "user",
					Content: json.RawMessage(`[
						{"type": "text", "text": "first part"},
						{"type": "image_url", "image_url": {"url": "https://example.com/x.jpg"}},
						{"type": "text", "text": "second part"}
					]`),
				},
			},
			want: "first part\nsecond part",
		},
		{
			name: "unguardable content shape returns empty",
			messages: []Message{
				{Role: "user", Content: json.RawMessage(`{"weird": "object"}`)},
			},
			want: "",
		},
		{
			name: "missing content returns empty",
			messages: []Message{
				{Role: "user"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatRequest{Messages: tt.messages}
			got := ExtractInputText(req)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractOutputText(t *testing.T) {
	tests := []struct {
		name    string
		choices []Choice
		want    string
	}{
		{
			name: "single choice with plain text",
			choices: []Choice{
				{Message: Message{Role: "assistant", Content: StringContent("Hello!")}},
			},
			want: "Hello!",
		},
		{
			name:    "no choices",
			choices: []Choice{},
			want:    "",
		},
		{
			name: "tool calls only with no content",
			choices: []Choice{
				{
					Message: Message{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{
								ID:   "call_abc",
								Type: "function",
								Function: FunctionCall{
									Name:      "get_weather",
									Arguments: `{"city":"SF"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
			want: "",
		},
		{
			name: "tool calls with explicit null content",
			choices: []Choice{
				{
					Message: Message{
						Role:    "assistant",
						Content: json.RawMessage(`null`),
						ToolCalls: []ToolCall{
							{
								ID:       "call_xyz",
								Type:     "function",
								Function: FunctionCall{Name: "ping", Arguments: "{}"},
							},
						},
					},
				},
			},
			want: "",
		},
		{
			name: "multi-part assistant content",
			choices: []Choice{
				{
					Message: Message{
						Role: "assistant",
						Content: json.RawMessage(`[
							{"type": "text", "text": "Here is your answer"}
						]`),
					},
				},
			},
			want: "Here is your answer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &ChatResponse{Choices: tt.choices}
			got := ExtractOutputText(resp)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatRequest_ToolsRoundTrip(t *testing.T) {
	// A representative function-calling request body. We unmarshal it into
	// our types, marshal it back out, and re-unmarshal to confirm nothing
	// was silently dropped on the way through.
	wire := `{
		"model": "openai/gpt-4o",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "What is the weather in SF?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"city\":\"SF\"}"
						}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_1",
				"name": "get_weather",
				"content": "{\"temp\":62,\"unit\":\"F\"}"
			}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get the current weather for a city.",
					"parameters": {
						"type": "object",
						"properties": {
							"city": {"type": "string"}
						},
						"required": ["city"]
					}
				}
			}
		],
		"tool_choice": "auto"
	}`

	var req ChatRequest
	if err := json.Unmarshal([]byte(wire), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.Model != "openai/gpt-4o" {
		t.Errorf("Model = %q, want openai/gpt-4o", req.Model)
	}
	if len(req.Messages) != 4 {
		t.Fatalf("Messages len = %d, want 4", len(req.Messages))
	}

	// Tool definitions preserved.
	if len(req.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(req.Tools))
	}
	if req.Tools[0].Type != "function" {
		t.Errorf("Tools[0].Type = %q, want function", req.Tools[0].Type)
	}
	if req.Tools[0].Function.Name != "get_weather" {
		t.Errorf("Tools[0].Function.Name = %q, want get_weather", req.Tools[0].Function.Name)
	}
	if len(req.Tools[0].Function.Parameters) == 0 {
		t.Error("Tools[0].Function.Parameters lost during unmarshal")
	}

	// tool_choice preserved as raw JSON.
	if string(req.ToolChoice) != `"auto"` {
		t.Errorf("ToolChoice = %s, want \"auto\"", req.ToolChoice)
	}

	// Assistant message tool_calls preserved.
	assistant := req.Messages[2]
	if assistant.Role != "assistant" {
		t.Errorf("Messages[2].Role = %q, want assistant", assistant.Role)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("Messages[2].ToolCalls len = %d, want 1", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCall ID = %q, want call_1", assistant.ToolCalls[0].ID)
	}
	if assistant.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("ToolCall function name = %q, want get_weather", assistant.ToolCalls[0].Function.Name)
	}
	if assistant.ToolCalls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("ToolCall arguments = %q, want {\"city\":\"SF\"}", assistant.ToolCalls[0].Function.Arguments)
	}

	// Tool result message preserved.
	tool := req.Messages[3]
	if tool.Role != "tool" {
		t.Errorf("Messages[3].Role = %q, want tool", tool.Role)
	}
	if tool.ToolCallID != "call_1" {
		t.Errorf("Messages[3].ToolCallID = %q, want call_1", tool.ToolCallID)
	}
	if tool.Name != "get_weather" {
		t.Errorf("Messages[3].Name = %q, want get_weather", tool.Name)
	}

	// Round-trip: marshal then unmarshal again, expect equivalent shape.
	out, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var req2 ChatRequest
	if err := json.Unmarshal(out, &req2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(req2.Tools) != 1 || req2.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools lost on round-trip: %s", out)
	}
	if len(req2.Messages) != 4 {
		t.Errorf("messages lost on round-trip: %s", out)
	}
	if len(req2.Messages[2].ToolCalls) != 1 {
		t.Errorf("tool_calls lost on round-trip: %s", out)
	}

	// Sanity: the user message should still extract via ExtractInputText.
	if got := ExtractInputText(&req2); got != "What is the weather in SF?" {
		t.Errorf("ExtractInputText = %q, want %q", got, "What is the weather in SF?")
	}
}

func TestStringContent(t *testing.T) {
	got := StringContent(`hello "world"`)
	if string(got) != `"hello \"world\""` {
		t.Errorf("StringContent = %s, want escaped JSON string", got)
	}
}
