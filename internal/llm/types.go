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
// Content is a plain string. Multi-part content (OpenAI vision blocks,
// structured tool-result arrays) is NOT yet supported — callers sending
// those shapes will lose the non-string fields on unmarshal. Changing
// Content to preserve structured shapes is a deferred decision.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant", "tool"
	Content string `json:"content"` // plain text

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

// ExtractInputText pulls the last user message content from a chat request.
// This is what gets validated by input guards.
func ExtractInputText(req *ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
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
	return resp.Choices[0].Message.Content
}
