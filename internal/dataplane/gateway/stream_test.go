package gateway

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cyacco/Sheeld/internal/shared/llm"
)

func TestSplitContent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		size int
	}{
		{"empty", "", 10},
		{"short", "hi", 10},
		{"exact", "abcdefghij", 10},
		{"words", "the quick brown fox jumps over the lazy dog", 10},
		{"no spaces", strings.Repeat("x", 25), 10},
		{"multibyte-safe concat", "héllo wörld héllo wörld héllo wörld", 8},
		{"spaceless multibyte", strings.Repeat("é", 30), 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pieces := splitContent(tt.in, tt.size)
			if strings.Join(pieces, "") != tt.in {
				t.Errorf("concatenation mismatch: %q -> %q", tt.in, strings.Join(pieces, ""))
			}
			for i, p := range pieces {
				if p == "" {
					t.Errorf("piece %d empty", i)
				}
				if !utf8.ValidString(p) {
					t.Errorf("piece %d splits a rune: %q", i, p)
				}
			}
		})
	}
	if pieces := splitContent("", 10); pieces != nil {
		t.Errorf("empty input should yield nil, got %v", pieces)
	}
}

func TestWriteSSEResponse(t *testing.T) {
	resp := &llm.ChatResponse{
		ID: "cmpl-1", Object: "chat.completion", Created: 123, Model: "m",
		Choices: []llm.Choice{{
			Index:        0,
			Message:      llm.Message{Role: "assistant", Content: "the quick brown fox jumps over the lazy dog and keeps on running far away"},
			FinishReason: "stop",
		}},
	}

	rec := httptest.NewRecorder()
	writeSSEResponse(rec, resp)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}

	var content strings.Builder
	var sawRole, sawFinish, sawDone bool
	var contentChunks int
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			sawDone = true
			continue
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			t.Fatalf("bad chunk %q: %v", payload, err)
		}
		if chunk.Object != "chat.completion.chunk" || chunk.ID != "cmpl-1" || chunk.Model != "m" {
			t.Errorf("chunk envelope wrong: %+v", chunk)
		}
		c := chunk.Choices[0]
		if c.Delta.Role == "assistant" {
			sawRole = true
		}
		if c.Delta.Content != "" {
			contentChunks++
			content.WriteString(c.Delta.Content)
		}
		if c.FinishReason != nil && *c.FinishReason == "stop" {
			sawFinish = true
		}
	}

	if !sawRole || !sawFinish || !sawDone {
		t.Errorf("missing events: role=%v finish=%v done=%v", sawRole, sawFinish, sawDone)
	}
	if contentChunks < 2 {
		t.Errorf("expected content split into multiple chunks, got %d", contentChunks)
	}
	if got := content.String(); got != resp.Choices[0].Message.Content {
		t.Errorf("reassembled content = %q", got)
	}
}
