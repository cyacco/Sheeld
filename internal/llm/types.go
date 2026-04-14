package llm

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

	// Stream enables Server-Sent Events streaming when true. Pointer so it
	// omits cleanly when the caller doesn't care.
	Stream *bool `json:"stream,omitempty"`
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

// ChatResponseChunk is a single SSE frame from a streaming chat completion.
// Matches OpenAI's `chat.completion.chunk` schema.
type ChatResponseChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
}

// ChunkChoice is a single choice within a streaming chunk. Delta is a
// partial Message — Content may be empty (e.g., the first chunk often only
// carries `role: "assistant"`).
type ChunkChoice struct {
	Index        int     `json:"index"`
	Delta        Message `json:"delta"`
	FinishReason *string `json:"finish_reason,omitempty"`
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
func ExtractOutputText(resp *ChatResponse) string {
	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content
	}
	return ""
}
