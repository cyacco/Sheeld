package llm

import (
	"encoding/json"
	"maps"
	"strings"
)

// The proxy speaks the OpenAI chat-completions schema, but only a handful of
// fields matter to the guardrail pipeline (messages, model, stream). Rather
// than enumerate every OpenAI field — tools, tool_choice, response_format,
// top_p, n, seed, stop, logprobs, stream_options, … which change over time —
// the request/response types capture unmodeled fields as raw JSON and emit
// them unchanged. This keeps Sheeld a true drop-in: function calling,
// structured outputs, and multimodal content pass through untouched.

// ChatRequest represents an OpenAI-compatible chat completion request.
type ChatRequest struct {
	// Model is the model name; the proxy overrides it with the source's
	// configured model before forwarding.
	Model string
	// Messages is the conversation history.
	Messages []Message
	// Stream requests an SSE response. Sheeld buffers the full pipeline
	// (transformers and guards see the complete response) and then replays
	// the approved text as chunks; the LLM gateway is always called
	// non-streaming.
	Stream bool

	// extra holds every other top-level field (tools, response_format,
	// temperature, top_p, …) so they round-trip to the provider verbatim.
	extra map[string]json.RawMessage
}

func (r *ChatRequest) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["model"]; ok {
		if err := json.Unmarshal(v, &r.Model); err != nil {
			return err
		}
	}
	if v, ok := raw["messages"]; ok {
		if err := json.Unmarshal(v, &r.Messages); err != nil {
			return err
		}
	}
	if v, ok := raw["stream"]; ok {
		if err := json.Unmarshal(v, &r.Stream); err != nil {
			return err
		}
	}
	delete(raw, "model")
	delete(raw, "messages")
	delete(raw, "stream")
	r.extra = raw
	return nil
}

func (r ChatRequest) MarshalJSON() ([]byte, error) {
	out := cloneExtra(r.extra)
	out["model"], _ = json.Marshal(r.Model)
	out["messages"], _ = json.Marshal(r.Messages)
	out["stream"], _ = json.Marshal(r.Stream)
	return json.Marshal(out)
}

// Message represents a single message in the conversation. Content is exposed
// as plain text for guards and transformers; when the original content was a
// multimodal array (or the message carries fields like tool_calls), those are
// preserved and re-emitted unless a transformer rewrote the text.
type Message struct {
	Role string
	// Content is the message text. For multimodal content arrays it is the
	// concatenation of the text parts (what guards evaluate and transformers
	// rewrite).
	Content string

	// contentRaw is the original JSON of a non-string content (array/object/
	// null); contentText is the text extracted from it at decode time. On
	// marshal, if Content is unchanged from contentText the raw form is
	// re-emitted; if a transformer changed it, the new string is emitted.
	contentRaw  json.RawMessage
	contentText string

	// extra holds other message fields (tool_calls, tool_call_id, name,
	// refusal, …) so assistant/tool messages round-trip intact.
	extra map[string]json.RawMessage
}

func (m *Message) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["role"]; ok {
		if err := json.Unmarshal(v, &m.Role); err != nil {
			return err
		}
	}
	if v, ok := raw["content"]; ok && len(v) > 0 {
		switch {
		case v[0] == '"':
			// Plain string content.
			if err := json.Unmarshal(v, &m.Content); err != nil {
				return err
			}
		case string(v) == "null":
			// Null content (e.g. an assistant message that only has
			// tool_calls): preserve it so it re-emits as null, not "".
			m.contentRaw = json.RawMessage("null")
			m.contentText = ""
		default:
			// Multimodal array or other shape: keep the raw form, expose the
			// text parts for the pipeline.
			m.contentRaw = append(json.RawMessage(nil), v...)
			m.Content = extractContentText(v)
			m.contentText = m.Content
		}
	}
	delete(raw, "role")
	delete(raw, "content")
	m.extra = raw
	return nil
}

func (m Message) MarshalJSON() ([]byte, error) {
	out := cloneExtra(m.extra)
	out["role"], _ = json.Marshal(m.Role)
	switch {
	case m.contentRaw != nil && m.Content == m.contentText:
		// Untouched multimodal/complex content: re-emit verbatim.
		out["content"] = m.contentRaw
	default:
		out["content"], _ = json.Marshal(m.Content)
	}
	return json.Marshal(out)
}

// ChatResponse represents an OpenAI-compatible chat completion response.
type ChatResponse struct {
	ID      string
	Object  string
	Created int64
	Model   string
	Choices []Choice
	Usage   Usage

	extra map[string]json.RawMessage
}

func (r *ChatResponse) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	unmarshalField(raw, "id", &r.ID)
	unmarshalField(raw, "object", &r.Object)
	unmarshalField(raw, "created", &r.Created)
	unmarshalField(raw, "model", &r.Model)
	unmarshalField(raw, "choices", &r.Choices)
	unmarshalField(raw, "usage", &r.Usage)
	for _, k := range []string{"id", "object", "created", "model", "choices", "usage"} {
		delete(raw, k)
	}
	r.extra = raw
	return nil
}

func (r ChatResponse) MarshalJSON() ([]byte, error) {
	out := cloneExtra(r.extra)
	out["id"], _ = json.Marshal(r.ID)
	out["object"], _ = json.Marshal(r.Object)
	out["created"], _ = json.Marshal(r.Created)
	out["model"], _ = json.Marshal(r.Model)
	out["choices"], _ = json.Marshal(r.Choices)
	out["usage"], _ = json.Marshal(r.Usage)
	return json.Marshal(out)
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int
	Message      Message
	FinishReason string

	extra map[string]json.RawMessage
}

func (c *Choice) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	unmarshalField(raw, "index", &c.Index)
	unmarshalField(raw, "message", &c.Message)
	unmarshalField(raw, "finish_reason", &c.FinishReason)
	for _, k := range []string{"index", "message", "finish_reason"} {
		delete(raw, k)
	}
	c.extra = raw
	return nil
}

func (c Choice) MarshalJSON() ([]byte, error) {
	out := cloneExtra(c.extra)
	out["index"], _ = json.Marshal(c.Index)
	out["message"], _ = json.Marshal(c.Message)
	out["finish_reason"], _ = json.Marshal(c.FinishReason)
	return json.Marshal(out)
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

// cloneExtra returns a shallow copy of an extra-fields map (never nil), so
// marshaling doesn't mutate the source struct's map.
func cloneExtra(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in)+6)
	maps.Copy(out, in)
	return out
}

// unmarshalField decodes raw[key] into dst if present, ignoring errors for
// absent keys.
func unmarshalField(raw map[string]json.RawMessage, key string, dst any) {
	if v, ok := raw[key]; ok {
		_ = json.Unmarshal(v, dst)
	}
}

// extractContentText concatenates the text parts of a multimodal content
// array (objects with {"type":"text","text":"..."}). Non-text parts (images,
// audio) contribute nothing to the guarded text but are preserved in the raw
// form. Returns "" if the shape isn't a recognizable parts array.
func extractContentText(raw json.RawMessage) string {
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(p.Text)
		}
	}
	return b.String()
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
