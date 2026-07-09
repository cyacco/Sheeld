// mock-llm is a tiny OpenAI-compatible chat-completions server used by the
// Docker Compose demo, so the full guardrail pipeline runs end to end with no
// provider API key. It accepts any model name and any API key and returns a
// canned completion. Point a source at a real provider (or your own gateway)
// to go live.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	// The data plane calls <base>/chat/completions; accept the /v1 prefix too
	// so the server also works as an OpenAI SDK base_url.
	mux.HandleFunc("/chat/completions", handleChat)
	mux.HandleFunc("/v1/chat/completions", handleChat)

	slog.Info("mock LLM server listening", "port", port)
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
		return
	}

	content := "Hello from Sheeld! This is a mock LLM response — the guardrail pipeline ran end to end. Point your source at a real OpenAI-compatible provider to go live."
	promptTokens := 0
	for _, m := range req.Messages {
		promptTokens += len(m.Content) / 4 // rough token estimate
	}

	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     promptTokens,
			"completion_tokens": len(content) / 4,
			"total_tokens":      promptTokens + len(content)/4,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
