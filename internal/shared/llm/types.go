package llm

import "strings"

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
}

// Message represents a single message in the conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // The message text
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

// SerializeMessages renders the full messages array as role-prefixed lines
// ("system: ...\nuser: ..."), in order. Used by guards with
// scope: all_messages to validate whole conversations.
func SerializeMessages(messages []Message) string {
	var b strings.Builder
	for i, m := range messages {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Content)
	}
	return b.String()
}

// ExtractOutputText pulls the assistant message content from a chat response.
// This is what gets validated by output guards.
func ExtractOutputText(resp *ChatResponse) string {
	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content
	}
	return ""
}
