package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/sheeld/sheeld/internal/shared/llm"
)

// streamChunkSize is the approximate number of bytes of content per SSE
// delta. The response is already fully buffered (and guard-approved) when
// streaming starts; chunking exists so streaming clients render
// incrementally.
const streamChunkSize = 48

// chatChunk is an OpenAI chat.completion.chunk SSE event.
type chatChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type chunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// writeSSEResponse replays a buffered chat completion as OpenAI-compatible
// SSE: a role delta, content deltas, a finish_reason event, and [DONE].
func writeSSEResponse(w http.ResponseWriter, resp *llm.ChatResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	emit := func(c chatChunk) {
		b, err := json.Marshal(c)
		if err != nil {
			return
		}
		w.Write([]byte("data: "))
		w.Write(b)
		w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	base := func(choices []chunkChoice) chatChunk {
		return chatChunk{
			ID:      resp.ID,
			Object:  "chat.completion.chunk",
			Created: resp.Created,
			Model:   resp.Model,
			Choices: choices,
		}
	}

	for _, choice := range resp.Choices {
		emit(base([]chunkChoice{{Index: choice.Index, Delta: chunkDelta{Role: choice.Message.Role}}}))
		for _, piece := range splitContent(choice.Message.Content, streamChunkSize) {
			emit(base([]chunkChoice{{Index: choice.Index, Delta: chunkDelta{Content: piece}}}))
		}
		finish := choice.FinishReason
		if finish == "" {
			finish = "stop"
		}
		emit(base([]chunkChoice{{Index: choice.Index, FinishReason: &finish}}))
	}

	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

// splitContent breaks text into ~size-byte pieces on word boundaries where
// possible. Concatenating the pieces always reproduces the input exactly.
func splitContent(text string, size int) []string {
	if text == "" {
		return nil
	}
	var pieces []string
	for len(text) > size {
		cut := size
		// Never split a UTF-8 rune: back up past continuation bytes,
		// otherwise json.Marshal would mangle the boundary.
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		// Prefer breaking after a space inside the window.
		if i := strings.LastIndexByte(text[:cut], ' '); i > 0 {
			cut = i + 1
		}
		if cut == 0 {
			cut = size // pathological input; take the bytes as-is
		}
		pieces = append(pieces, text[:cut])
		text = text[cut:]
	}
	return append(pieces, text)
}
