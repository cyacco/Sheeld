package llm

import "encoding/json"

// ChatRequest represents an OpenAI-compatible chat completion request.
// This is the format LiteLLM accepts for all providers.
type ChatRequest struct {
	// Model in LiteLLM format: "openai/gpt-4o", "anthropic/claude-sonnet-4-20250514", etc.
	Model string `json:"model"`

	// Messages is the conversation history.
	Messages []Message `json:"messages"`

	// Temperature controls randomness (0.0 - 2.0).
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens limits the response length.
	MaxTokens *int `json:"max_tokens,omitempty"`

	// TopP nucleus sampling parameter.
	TopP *float64 `json:"top_p,omitempty"`

	// Stop sequences.
	Stop []string `json:"stop,omitempty"`

	// Tools is the list of tools (functions) the model may call.
	// Passed through verbatim to the upstream provider via LiteLLM.
	Tools []Tool `json:"tools,omitempty"`

	// ToolChoice controls which (if any) tool the model is forced to call.
	// May be a string ("auto", "none", "required") or an object — kept as
	// raw JSON so we don't lose fidelity on unmarshal/marshal round-trips.
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
}

// Message represents a single message in the conversation.
//
// Content is stored as raw JSON to losslessly preserve both plain-string
// content (the classic shape) and OpenAI's structured multi-part content
// arrays (used for vision and tool results). Use ExtractInputText /
// ExtractOutputText (or unmarshal Content yourself) to read it.
type Message struct {
	Role    string          `json:"role"`              // "system", "user", "assistant", "tool"
	Content json.RawMessage `json:"content,omitempty"` // string OR []ContentPart

	// Name is an optional author name (e.g., for tool messages it identifies
	// the tool, for user/assistant messages it can disambiguate participants).
	Name string `json:"name,omitempty"`

	// ToolCalls are tool/function calls emitted by an assistant message.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is set on role="tool" messages and references the
	// assistant tool_call this message is responding to.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall is a single tool/function invocation requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function-call payload inside a ToolCall.
// Arguments is a JSON string per the OpenAI wire format.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is a tool definition the caller exposes to the model.
type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a callable function for the model.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ChatResponse represents an OpenAI-compatible chat completion response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorResponse represents an error from the LLM gateway.
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// StringContent wraps a plain string into the json.RawMessage shape expected
// by Message.Content. Useful for tests and for callers constructing a
// ChatRequest in Go (rather than unmarshaling from the wire).
func StringContent(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// contentPart is the multi-part content shape used by OpenAI's vision and
// tool-result formats. Only the text variant is interesting to guards today.
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// messageText extracts a flat text representation from a message's Content.
// Plain-string content is returned as-is. Multi-part content has all
// {type:"text"} parts concatenated with newlines. Anything else returns "".
func messageText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	// Try string first — this is the common case.
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}

	// Try multi-part array.
	var parts []contentPart
	if err := json.Unmarshal(content, &parts); err == nil {
		var out string
		for _, p := range parts {
			if p.Type != "text" {
				continue
			}
			if out != "" {
				out += "\n"
			}
			out += p.Text
		}
		return out
	}

	// Unknown / unguardable shape.
	return ""
}

// ExtractInputText pulls the last user message content from a chat request.
// This is what gets validated by input guards.
//
// Supports both plain-string content and OpenAI multi-part content arrays
// (e.g. vision messages). For multi-part content, all text parts are
// concatenated with newlines; non-text parts (images, etc.) are ignored.
func ExtractInputText(req *ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return messageText(req.Messages[i].Content)
		}
	}
	return ""
}

// ExtractOutputText pulls the assistant message content from a chat response.
// This is what gets validated by output guards.
//
// If the assistant response is purely tool calls with no text content,
// returns "". Guards over tool-call arguments are a planned follow-up.
func ExtractOutputText(resp *ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return messageText(resp.Choices[0].Message.Content)
}
