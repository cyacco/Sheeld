//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sheeld/sheeld/internal/controlplane/api"
	"github.com/sheeld/sheeld/internal/controlplane/config"
	"github.com/sheeld/sheeld/internal/controlplane/db"
	"github.com/sheeld/sheeld/internal/controlplane/db/generated"
	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/proxy"
	"github.com/sheeld/sheeld/internal/controlplane/service"
)

// Package-level test infrastructure
var (
	testServer *httptest.Server
	pool       *pgxpool.Pool
	pgCtr      *postgres.PostgresContainer

	// mockLLMResponseContent controls what the mock LLM server returns.
	mockLLMResponseContent   = "Hello! I'm a helpful assistant."
	mockLLMResponseContentMu sync.Mutex
)

func setMockLLMResponse(content string) {
	mockLLMResponseContentMu.Lock()
	defer mockLLMResponseContentMu.Unlock()
	mockLLMResponseContent = content
}

func getMockLLMResponse() string {
	mockLLMResponseContentMu.Lock()
	defer mockLLMResponseContentMu.Unlock()
	return mockLLMResponseContent
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	// 1. Start PostgreSQL container
	var err error
	pgCtr, err = postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("sheeld_test"),
		postgres.WithUsername("sheeld"),
		postgres.WithPassword("sheeld_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	// 2. Connect and run migrations
	pool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	if err := db.RunMigrations(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	// 3. Start mock LLM server
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/chat/completions") {
			resp := llm.ChatResponse{
				ID:      "chatcmpl-test-123",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "gpt-4o",
				Choices: []llm.Choice{
					{
						Index: 0,
						Message: llm.Message{
							Role:    "assistant",
							Content: getMockLLMResponse(),
						},
						FinishReason: "stop",
					},
				},
				Usage: llm.Usage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))

	// 4. Build full router
	encryptionKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cfg := &config.Config{
		Port:               0,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       60 * time.Second,
		ShutdownTimeout:    10 * time.Second,
		DatabaseURL:        connStr,
		JWTSecret:          "test-jwt-secret-for-integration-tests",
		JWTExpiration:      24 * time.Hour,
		EncryptionKey:      encryptionKey,
		LLMGatewayURL:      mockLLM.URL,
		LLMRequestTimeout:  10 * time.Second,
		RateLimitRPS:       1000,
		RateLimitBurst:     2000,
		MaxBodyBytes:       1048576,
		ProxyTimeout:       60 * time.Second,
		CORSAllowedOrigins: []string{"*"},
	}

	queries := generated.New(pool)
	authService := service.NewAuthService(queries, cfg.JWTSecret, cfg.JWTExpiration)
	sourceService := service.NewSourceService(queries, cfg.EncryptionKey)
	guardrailService := service.NewGuardrailService(queries)

	guardRegistry := guard.NewRegistry()
	guardEngine := guard.NewEngine(guardRegistry)
	llmClient := llm.NewClient(cfg.LLMGatewayURL, cfg.LLMRequestTimeout)
	proxyService := proxy.NewProxy(queries, guardEngine, llmClient, cfg.EncryptionKey)

	router := api.NewRouter(cfg, pool, authService, sourceService, guardrailService, proxyService, queries)
	testServer = httptest.NewServer(router)

	// 5. Run tests
	code := m.Run()

	// 6. Cleanup
	testServer.Close()
	mockLLM.Close()
	pool.Close()
	pgCtr.Terminate(ctx)

	os.Exit(code)
}

// --- Helpers ---

func doRequest(t *testing.T, method, path string, body interface{}, authToken string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, testServer.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authToken != "" {
		if strings.HasPrefix(authToken, "shld_") {
			req.Header.Set("Authorization", "Bearer "+authToken)
		} else {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func expectStatus(t *testing.T, resp *http.Response, wantCode int) {
	t.Helper()
	if resp.StatusCode != wantCode {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d; body: %s", wantCode, resp.StatusCode, string(body))
	}
}

func decodeBody(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode body: %v; raw: %s", err, string(body))
	}
}

func registerUser(t *testing.T, orgName, email, password string) string {
	t.Helper()
	resp := doRequest(t, "POST", "/v1/auth/register", map[string]string{
		"org_name": orgName,
		"email":    email,
		"password": password,
	}, "")
	expectStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected token in register response, got: %v", result)
	}
	return token
}

func loginUser(t *testing.T, email, password string) string {
	t.Helper()
	resp := doRequest(t, "POST", "/v1/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, "")
	expectStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected token in login response, got: %v", result)
	}
	return token
}

func createAPIKey(t *testing.T, token, name string) string {
	t.Helper()
	resp := doRequest(t, "POST", "/v1/auth/api-keys", map[string]string{
		"name": name,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	rawKey, ok := result["raw_key"].(string)
	if !ok || rawKey == "" {
		t.Fatalf("expected raw_key in create API key response, got: %v", result)
	}
	return rawKey
}

func createSource(t *testing.T, token string, name, route string) string {
	t.Helper()
	resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
		"name":          name,
		"route":         route,
		"llm_provider":  "openai",
		"llm_model":     "gpt-4o",
		"llm_api_key":   "sk-test-key-12345",
		"pass_criteria": "all",
		"enabled":       true,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result["id"].(string)
}

func createGuardrail(t *testing.T, token string, params map[string]interface{}) string {
	t.Helper()
	resp := doRequest(t, "POST", "/v1/guardrails", params, token)
	expectStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected guardrail ID, got: %v", result)
	}
	return id
}

func attachGuardrail(t *testing.T, token, guardrailID, sourceID string) {
	t.Helper()
	resp := doRequest(t, "POST", "/v1/guardrails/"+guardrailID+"/sources", map[string]interface{}{
		"source_id": sourceID,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

// --- Test Cases ---

func TestHealthz(t *testing.T) {
	resp := doRequest(t, "GET", "/healthz", nil, "")
	expectStatus(t, resp, http.StatusOK)
	var body map[string]string
	decodeBody(t, resp, &body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
	if body["db"] != "connected" {
		t.Errorf("expected db connected, got %q", body["db"])
	}
}

func TestAuth(t *testing.T) {
	t.Run("register happy path", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/register", map[string]string{
			"org_name": "Auth Test Org",
			"email":    "authtest@example.com",
			"password": "strongpassword123",
		}, "")
		expectStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["token"] == nil {
			t.Error("expected token in response")
		}
		if result["organization"] == nil {
			t.Error("expected organization in response")
		}
		if result["user"] == nil {
			t.Error("expected user in response")
		}
	})

	t.Run("register missing email", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/register", map[string]string{
			"org_name": "Missing Email Org",
			"password": "strongpassword123",
		}, "")
		expectStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("register short password", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/register", map[string]string{
			"org_name": "Short PW Org",
			"email":    "shortpw@example.com",
			"password": "short",
		}, "")
		expectStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("register duplicate email", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/register", map[string]string{
			"org_name": "Dup Org",
			"email":    "authtest@example.com",
			"password": "strongpassword123",
		}, "")
		expectStatus(t, resp, http.StatusInternalServerError)
	})

	t.Run("login happy path", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/login", map[string]string{
			"email":    "authtest@example.com",
			"password": "strongpassword123",
		}, "")
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["token"] == nil {
			t.Error("expected token in response")
		}
	})

	t.Run("login wrong password", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/login", map[string]string{
			"email":    "authtest@example.com",
			"password": "wrongpassword",
		}, "")
		expectStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("login nonexistent user", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/auth/login", map[string]string{
			"email":    "nonexistent@example.com",
			"password": "anypassword123",
		}, "")
		expectStatus(t, resp, http.StatusUnauthorized)
	})

	t.Run("api key lifecycle", func(t *testing.T) {
		token := loginUser(t, "authtest@example.com", "strongpassword123")

		// Create API key
		resp := doRequest(t, "POST", "/v1/auth/api-keys", map[string]string{
			"name": "test-key",
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var createResult map[string]interface{}
		decodeBody(t, resp, &createResult)
		rawKey, ok := createResult["raw_key"].(string)
		if !ok || !strings.HasPrefix(rawKey, "shld_") {
			t.Fatalf("expected raw_key starting with shld_, got %q", rawKey)
		}

		// List API keys
		resp = doRequest(t, "GET", "/v1/auth/api-keys", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var keys []interface{}
		decodeBody(t, resp, &keys)
		if len(keys) == 0 {
			t.Error("expected at least one API key")
		}

		// Extract key ID for revocation
		apiKey := createResult["api_key"].(map[string]interface{})
		keyID := apiKey["id"].(string)

		// Revoke API key
		resp = doRequest(t, "DELETE", "/v1/auth/api-keys/"+keyID, nil, token)
		expectStatus(t, resp, http.StatusOK)
	})

	t.Run("unauthenticated api-keys", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/auth/api-keys", nil, "")
		expectStatus(t, resp, http.StatusUnauthorized)
	})
}

func TestSources(t *testing.T) {
	token := registerUser(t, "Sources Test Org", "sources@example.com", "strongpassword123")

	var sourceID string

	t.Run("create source", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
			"name":          "Test Source",
			"route":         "test-source",
			"description":   "A test source",
			"llm_provider":  "openai",
			"llm_model":     "gpt-4o",
			"llm_api_key":   "sk-test-key-12345",
			"pass_criteria": "all",
			"enabled":       true,
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)

		sourceID = result["id"].(string)
		if sourceID == "" {
			t.Fatal("expected source ID")
		}
		if result["name"] != "Test Source" {
			t.Errorf("expected name 'Test Source', got %v", result["name"])
		}
		if result["route"] != "test-source" {
			t.Errorf("expected route 'test-source', got %v", result["route"])
		}
		// llm_api_key should NOT be exposed
		if result["llm_api_key"] != nil {
			t.Error("llm_api_key should not be exposed in response")
		}
	})

	t.Run("list sources", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/sources", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var sources []interface{}
		decodeBody(t, resp, &sources)
		if len(sources) == 0 {
			t.Error("expected at least one source")
		}
	})

	t.Run("get source by ID", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/sources/"+sourceID, nil, token)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["id"] != sourceID {
			t.Errorf("expected id %s, got %v", sourceID, result["id"])
		}
	})

	t.Run("update source", func(t *testing.T) {
		resp := doRequest(t, "PUT", "/v1/sources/"+sourceID, map[string]interface{}{
			"name":          "Updated Source",
			"route":         "test-source",
			"llm_provider":  "openai",
			"llm_model":     "gpt-4o",
			"llm_api_key":   "sk-test-key-12345",
			"pass_criteria": "all",
			"enabled":       true,
		}, token)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["name"] != "Updated Source" {
			t.Errorf("expected name 'Updated Source', got %v", result["name"])
		}
	})

	t.Run("delete source", func(t *testing.T) {
		// Create a separate source to delete
		resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
			"name":          "To Delete",
			"route":         "to-delete",
			"llm_provider":  "openai",
			"llm_model":     "gpt-4o",
			"llm_api_key":   "sk-test-key-12345",
			"pass_criteria": "all",
			"enabled":       true,
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var created map[string]interface{}
		decodeBody(t, resp, &created)
		delID := created["id"].(string)

		resp = doRequest(t, "DELETE", "/v1/sources/"+delID, nil, token)
		expectStatus(t, resp, http.StatusOK)

		// Subsequent GET should fail
		resp = doRequest(t, "GET", "/v1/sources/"+delID, nil, token)
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 404 or 500 after delete, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("org isolation", func(t *testing.T) {
		token2 := registerUser(t, "Other Org", "other@example.com", "strongpassword123")
		resp := doRequest(t, "GET", "/v1/sources/"+sourceID, nil, token2)
		// Second org should not be able to see first org's source
		if resp.StatusCode == http.StatusOK {
			t.Error("expected second org to NOT see first org's source")
		}
		resp.Body.Close()
	})
}

func TestGuardrails(t *testing.T) {
	token := registerUser(t, "Guardrails Test Org", "guardrails@example.com", "strongpassword123")

	var blocklist1ID, regex1ID string

	t.Run("create blocklist guardrail", func(t *testing.T) {
		blocklist1ID = createGuardrail(t, token, map[string]interface{}{
			"name":       "Block Bad Words",
			"guard_type": "blocklist",
			"phase":      "input",
			"config": map[string]interface{}{
				"words": []string{"forbidden", "banned"},
			},
			"enabled": true,
		})
	})

	t.Run("create regex guardrail", func(t *testing.T) {
		regex1ID = createGuardrail(t, token, map[string]interface{}{
			"name":       "Block SSN",
			"guard_type": "regex",
			"phase":      "input",
			"config": map[string]interface{}{
				"pattern": `\d{3}-\d{2}-\d{4}`,
			},
			"enabled": true,
		})
	})

	t.Run("list guardrails (org-scoped)", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/guardrails", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var guardrails []interface{}
		decodeBody(t, resp, &guardrails)
		if len(guardrails) < 2 {
			t.Errorf("expected at least 2 guardrails, got %d", len(guardrails))
		}
	})

	t.Run("get guardrail by ID", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/guardrails/"+blocklist1ID, nil, token)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["id"] != blocklist1ID {
			t.Errorf("expected id %s, got %v", blocklist1ID, result["id"])
		}
		if result["organization_id"] == nil {
			t.Error("expected organization_id in response")
		}
	})

	t.Run("update guardrail", func(t *testing.T) {
		resp := doRequest(t, "PUT", "/v1/guardrails/"+blocklist1ID, map[string]interface{}{
			"name":       "Updated Block Bad Words",
			"guard_type": "blocklist",
			"phase":      "input",
			"config": map[string]interface{}{
				"words": []string{"forbidden", "banned", "evil"},
			},
			"enabled": true,
		}, token)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["name"] != "Updated Block Bad Words" {
			t.Errorf("expected updated name, got %v", result["name"])
		}
	})

	t.Run("delete guardrail", func(t *testing.T) {
		resp := doRequest(t, "DELETE", "/v1/guardrails/"+regex1ID, nil, token)
		expectStatus(t, resp, http.StatusOK)

		// Subsequent GET should fail
		resp = doRequest(t, "GET", "/v1/guardrails/"+regex1ID, nil, token)
		if resp.StatusCode == http.StatusOK {
			t.Error("expected guardrail to be deleted")
		}
		resp.Body.Close()
	})

	t.Run("default phase is input", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/guardrails", map[string]interface{}{
			"name":       "No Phase Specified",
			"guard_type": "blocklist",
			"config": map[string]interface{}{
				"words": []string{"test"},
			},
			"enabled": true,
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["phase"] != "input" {
			t.Errorf("expected default phase 'input', got %v", result["phase"])
		}
	})

	t.Run("attach and detach", func(t *testing.T) {
		sourceID := createSource(t, token, "Attach Source", "attach-source")

		// Attach guardrail to source
		attachGuardrail(t, token, blocklist1ID, sourceID)

		// List guardrails for source
		resp := doRequest(t, "GET", "/v1/sources/"+sourceID+"/guardrails", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var guardrails []interface{}
		decodeBody(t, resp, &guardrails)
		if len(guardrails) != 1 {
			t.Fatalf("expected 1 guardrail attached, got %d", len(guardrails))
		}

		// List sources for guardrail
		resp = doRequest(t, "GET", "/v1/guardrails/"+blocklist1ID+"/sources", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var sources []interface{}
		decodeBody(t, resp, &sources)
		if len(sources) != 1 {
			t.Fatalf("expected 1 source attached, got %d", len(sources))
		}

		// Detach
		resp = doRequest(t, "DELETE", "/v1/guardrails/"+blocklist1ID+"/sources/"+sourceID, nil, token)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		// Verify empty
		resp = doRequest(t, "GET", "/v1/sources/"+sourceID+"/guardrails", nil, token)
		expectStatus(t, resp, http.StatusOK)
		decodeBody(t, resp, &guardrails)
		if len(guardrails) != 0 {
			t.Errorf("expected 0 guardrails after detach, got %d", len(guardrails))
		}
	})

	t.Run("reuse guardrail across sources", func(t *testing.T) {
		sourceA := createSource(t, token, "Reuse Source A", "reuse-a")
		sourceB := createSource(t, token, "Reuse Source B", "reuse-b")

		attachGuardrail(t, token, blocklist1ID, sourceA)
		attachGuardrail(t, token, blocklist1ID, sourceB)

		// Both sources should list the guardrail
		resp := doRequest(t, "GET", "/v1/sources/"+sourceA+"/guardrails", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var guardsA []interface{}
		decodeBody(t, resp, &guardsA)
		if len(guardsA) != 1 {
			t.Errorf("expected 1 guardrail on source A, got %d", len(guardsA))
		}

		resp = doRequest(t, "GET", "/v1/sources/"+sourceB+"/guardrails", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var guardsB []interface{}
		decodeBody(t, resp, &guardsB)
		if len(guardsB) != 1 {
			t.Errorf("expected 1 guardrail on source B, got %d", len(guardsB))
		}

		// Guardrail should list both sources
		resp = doRequest(t, "GET", "/v1/guardrails/"+blocklist1ID+"/sources", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var sources []interface{}
		decodeBody(t, resp, &sources)
		if len(sources) != 2 {
			t.Errorf("expected 2 sources attached, got %d", len(sources))
		}
	})

	t.Run("deleting source does not delete guardrail", func(t *testing.T) {
		sourceID := createSource(t, token, "Ephemeral Source", "ephemeral-source")
		gID := createGuardrail(t, token, map[string]interface{}{
			"name":       "Survivor Guard",
			"guard_type": "blocklist",
			"phase":      "input",
			"config":     map[string]interface{}{"words": []string{"x"}},
			"enabled":    true,
		})
		attachGuardrail(t, token, gID, sourceID)

		// Delete the source
		resp := doRequest(t, "DELETE", "/v1/sources/"+sourceID, nil, token)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		// Guardrail should still exist
		resp = doRequest(t, "GET", "/v1/guardrails/"+gID, nil, token)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

func TestProxy(t *testing.T) {
	token := registerUser(t, "Proxy Test Org", "proxy@example.com", "strongpassword123")

	// Reset mock LLM to default response
	setMockLLMResponse("Hello! I'm a helpful assistant.")

	t.Run("happy path no guards", func(t *testing.T) {
		createSource(t, token, "Proxy Happy", "proxy-happy")
		apiKey := createAPIKey(t, token, "proxy-test-key")

		resp := doRequest(t, "POST", "/v1/proxy/proxy-happy", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, world!"},
			},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["status"] != "pass" {
			t.Errorf("expected status 'pass', got %v", result["status"])
		}
		if result["llm_response"] == nil {
			t.Error("expected llm_response in result")
		}
	})

	t.Run("input guard rejects", func(t *testing.T) {
		sourceID := createSource(t, token, "Proxy Input Guard", "proxy-input-guard")
		gID := createGuardrail(t, token, map[string]interface{}{
			"name":       "Input Blocklist",
			"guard_type": "blocklist",
			"phase":      "input",
			"config":     map[string]interface{}{"words": []string{"forbidden"}},
			"enabled":    true,
		})
		attachGuardrail(t, token, gID, sourceID)

		apiKey := createAPIKey(t, token, "input-guard-key")

		resp := doRequest(t, "POST", "/v1/proxy/proxy-input-guard", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": "This message contains forbidden content"},
			},
		}, apiKey)
		expectStatus(t, resp, http.StatusForbidden)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["status"] != "rejected" {
			t.Errorf("expected status 'rejected', got %v", result["status"])
		}
		if result["phase"] != "input" {
			t.Errorf("expected phase 'input', got %v", result["phase"])
		}
		if result["llm_response"] != nil {
			t.Error("expected no llm_response on input rejection")
		}
	})

	t.Run("output guard rejects", func(t *testing.T) {
		setMockLLMResponse("Here is some badword content for you.")

		sourceID := createSource(t, token, "Proxy Output Guard", "proxy-output-guard")
		gID := createGuardrail(t, token, map[string]interface{}{
			"name":       "Output Blocklist",
			"guard_type": "blocklist",
			"phase":      "output",
			"config":     map[string]interface{}{"words": []string{"badword"}},
			"enabled":    true,
		})
		attachGuardrail(t, token, gID, sourceID)

		apiKey := createAPIKey(t, token, "output-guard-key")

		resp := doRequest(t, "POST", "/v1/proxy/proxy-output-guard", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": "Tell me something nice"},
			},
		}, apiKey)
		expectStatus(t, resp, http.StatusForbidden)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["status"] != "rejected" {
			t.Errorf("expected status 'rejected', got %v", result["status"])
		}
		if result["phase"] != "output" {
			t.Errorf("expected phase 'output', got %v", result["phase"])
		}

		setMockLLMResponse("Hello! I'm a helpful assistant.")
	})

	t.Run("all guards pass", func(t *testing.T) {
		setMockLLMResponse("This is a clean response.")

		sourceID := createSource(t, token, "Proxy All Pass", "proxy-all-pass")

		inputGID := createGuardrail(t, token, map[string]interface{}{
			"name":       "Input Blocklist Pass",
			"guard_type": "blocklist",
			"phase":      "input",
			"config":     map[string]interface{}{"words": []string{"forbidden"}},
			"enabled":    true,
		})
		attachGuardrail(t, token, inputGID, sourceID)

		outputGID := createGuardrail(t, token, map[string]interface{}{
			"name":       "Output Blocklist Pass",
			"guard_type": "blocklist",
			"phase":      "output",
			"config":     map[string]interface{}{"words": []string{"badword"}},
			"enabled":    true,
		})
		attachGuardrail(t, token, outputGID, sourceID)

		apiKey := createAPIKey(t, token, "all-pass-key")

		resp := doRequest(t, "POST", "/v1/proxy/proxy-all-pass", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, tell me something nice"},
			},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["status"] != "pass" {
			t.Errorf("expected status 'pass', got %v", result["status"])
		}
		if result["llm_response"] == nil {
			t.Error("expected llm_response in result")
		}

		setMockLLMResponse("Hello! I'm a helpful assistant.")
	})

	t.Run("invalid API key", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/proxy-happy", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}, "shld_invalidkey12345678901234567890")
		expectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})
}

func TestAuditLogs(t *testing.T) {
	// Register a user and create a source + proxy request to generate audit logs
	token := registerUser(t, "Audit Test Org", "audit@example.com", "strongpassword123")

	setMockLLMResponse("Clean response for audit test.")

	resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
		"name":          "Audit Source",
		"route":         "audit-source",
		"llm_provider":  "openai",
		"llm_model":     "gpt-4o",
		"llm_api_key":   "sk-test-key-12345",
		"pass_criteria": "all",
		"enabled":       true,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	var source map[string]interface{}
	decodeBody(t, resp, &source)
	sourceID := source["id"].(string)

	apiKey := createAPIKey(t, token, "audit-key")

	// Make a few proxy requests to generate audit logs
	for i := 0; i < 3; i++ {
		resp = doRequest(t, "POST", "/v1/proxy/audit-source", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": fmt.Sprintf("Message %d", i)},
			},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}

	setMockLLMResponse("Hello! I'm a helpful assistant.")

	t.Run("list audit logs", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/audit-logs", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var logs []interface{}
		decodeBody(t, resp, &logs)
		if len(logs) == 0 {
			t.Error("expected non-empty audit logs")
		}
	})

	t.Run("filter by source_id", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/audit-logs?source_id="+sourceID, nil, token)
		expectStatus(t, resp, http.StatusOK)
		var logs []interface{}
		decodeBody(t, resp, &logs)
		if len(logs) == 0 {
			t.Error("expected audit logs for source")
		}
		// All returned logs should be for our source
		for _, log := range logs {
			logMap := log.(map[string]interface{})
			if logMap["source_id"] != sourceID {
				t.Errorf("expected source_id %s, got %v", sourceID, logMap["source_id"])
			}
		}
	})

	t.Run("pagination limit", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/audit-logs?limit=1", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var logs []interface{}
		decodeBody(t, resp, &logs)
		if len(logs) != 1 {
			t.Errorf("expected exactly 1 log with limit=1, got %d", len(logs))
		}
	})
}

func TestModels(t *testing.T) {
	token := registerUser(t, "Models Test Org", "models@example.com", "strongpassword123")

	t.Run("list all", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/models", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var models []interface{}
		decodeBody(t, resp, &models)
		if len(models) != 11 {
			t.Errorf("expected 11 models, got %d", len(models))
		}
	})

	t.Run("filter openai", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/models?provider=openai", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var models []interface{}
		decodeBody(t, resp, &models)
		if len(models) != 8 {
			t.Errorf("expected 8 OpenAI models, got %d", len(models))
		}
		for _, m := range models {
			model := m.(map[string]interface{})
			if model["provider"] != "openai" {
				t.Errorf("expected provider openai, got %v", model["provider"])
			}
		}
	})

	t.Run("filter anthropic", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/models?provider=anthropic", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var models []interface{}
		decodeBody(t, resp, &models)
		if len(models) != 3 {
			t.Errorf("expected 3 Anthropic models, got %d", len(models))
		}
		for _, m := range models {
			model := m.(map[string]interface{})
			if model["provider"] != "anthropic" {
				t.Errorf("expected provider anthropic, got %v", model["provider"])
			}
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/models", nil, "")
		expectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})
}
