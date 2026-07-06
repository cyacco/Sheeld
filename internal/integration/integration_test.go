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
	"github.com/sheeld/sheeld/internal/controlplane/service"
	"github.com/sheeld/sheeld/internal/dataplane/auditstore"
	"github.com/sheeld/sheeld/internal/dataplane/backendconfig"
	dpconfig "github.com/sheeld/sheeld/internal/dataplane/config"
	dpdb "github.com/sheeld/sheeld/internal/dataplane/db"
	dpgenerated "github.com/sheeld/sheeld/internal/dataplane/db/generated"
	"github.com/sheeld/sheeld/internal/dataplane/gateway"
	"github.com/sheeld/sheeld/internal/dataplane/processor"
	"github.com/sheeld/sheeld/internal/shared/guard"
	"github.com/sheeld/sheeld/internal/shared/llm"
	"github.com/sheeld/sheeld/internal/shared/transform"
)

// Package-level test infrastructure
var (
	testServer  *httptest.Server // control plane
	dpServer    *httptest.Server // data plane
	poller      *backendconfig.Poller
	auditWriter *auditstore.Writer
	pool        *pgxpool.Pool
	pgCtr       *postgres.PostgresContainer

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

var (
	mockLLMLastRequest   llm.ChatRequest
	mockLLMLastRequestMu sync.Mutex
)

func setMockLLMLastRequest(req llm.ChatRequest) {
	mockLLMLastRequestMu.Lock()
	defer mockLLMLastRequestMu.Unlock()
	mockLLMLastRequest = req
}

func getMockLLMLastRequest() llm.ChatRequest {
	mockLLMLastRequestMu.Lock()
	defer mockLLMLastRequestMu.Unlock()
	return mockLLMLastRequest
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
			var req llm.ChatRequest
			json.NewDecoder(r.Body).Decode(&req)
			setMockLLMLastRequest(req)
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

	// 4. Create a second database for the data plane in the same container
	// and run its migrations.
	if _, err := pool.Exec(ctx, "CREATE DATABASE sheeld_dp_test"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create data-plane database: %v\n", err)
		os.Exit(1)
	}
	dpConnStr := strings.Replace(connStr, "/sheeld_test", "/sheeld_dp_test", 1)
	dpPool, err := pgxpool.New(ctx, dpConnStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to data-plane database: %v\n", err)
		os.Exit(1)
	}
	if err := dpdb.RunMigrations(ctx, dpPool); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run data-plane migrations: %v\n", err)
		os.Exit(1)
	}

	// 5. Build the two routers. The data plane starts first so its URL can
	// be configured on the control plane; the poller (which needs the
	// control-plane URL) is created after both are up.
	encryptionKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const dataPlaneToken = "test-dataplane-token"

	dpCfg := &dpconfig.Config{
		ControlPlaneURL:   "set-below",
		Token:             dataPlaneToken,
		PollInterval:      time.Hour, // tests refresh explicitly via FetchOnce
		AllowInsecureCP:   true,
		LLMRequestTimeout: 10 * time.Second,
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
		MaxBodyBytes:      1048576,
		ProxyTimeout:      60 * time.Second,
	}

	dpQueries := dpgenerated.New(dpPool)
	auditWriter = auditstore.NewWriter(dpQueries)
	store := backendconfig.NewStore()
	guardRegistry := guard.NewRegistry()
	// The integration suite registers the test transformer type; prod
	// registries ship empty in v1.
	transformRegistry := transform.NewRegistry()
	transformRegistry.Register("test_replace", transform.TestReplaceFactory)
	guardEngine := guard.NewEngine(guardRegistry)
	llmClient := llm.NewClient(mockLLM.URL, dpCfg.LLMRequestTimeout)
	proc := processor.NewProcessor(store, guardEngine, llmClient, auditWriter)
	dpServer = httptest.NewServer(gateway.NewRouter(dpCfg, store, proc, auditstore.NewHandler(dpQueries)))

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
		DataPlaneToken:     dataPlaneToken,
		DataPlaneURL:       dpServer.URL,
	}

	queries := generated.New(pool)
	authService := service.NewAuthService(queries, cfg.JWTSecret, cfg.JWTExpiration)
	sourceService := service.NewSourceService(queries, cfg.EncryptionKey)
	guardrailService := service.NewGuardrailService(queries)
	transformerService := service.NewTransformerService(queries, pool, transformRegistry)

	router := api.NewRouter(cfg, pool, authService, sourceService, guardrailService, transformerService, queries)
	testServer = httptest.NewServer(router)

	// Poller against the live control-plane test server. Tests trigger
	// refreshes explicitly via refreshConfig.
	poller = backendconfig.NewPoller(testServer.URL, dataPlaneToken, dpCfg.PollInterval, store, guardRegistry, transformRegistry)

	// 6. Run tests
	code := m.Run()

	// 7. Cleanup
	auditWriter.Close()
	testServer.Close()
	dpServer.Close()
	mockLLM.Close()
	dpPool.Close()
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

	// Proxy requests are served by the data plane; sync its config first so
	// control-plane changes made earlier in the test are visible.
	baseURL := testServer.URL
	if strings.HasPrefix(path, "/v1/proxy") {
		refreshConfig(t)
		baseURL = dpServer.URL
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
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

// refreshConfig synchronously pulls the latest workspace config into the
// data plane's store.
func refreshConfig(t *testing.T) {
	t.Helper()
	if err := poller.FetchOnce(context.Background()); err != nil {
		t.Fatalf("refreshing workspace config: %v", err)
	}
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
		if got := resp.Header.Get("X-Sheeld-Status"); got != "pass" {
			t.Errorf("expected X-Sheeld-Status 'pass', got %q", got)
		}
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		// Response is the raw OpenAI chat completion
		if result["object"] != "chat.completion" {
			t.Errorf("expected raw chat.completion response, got %v", result["object"])
		}
		if result["choices"] == nil {
			t.Error("expected choices in raw LLM response")
		}
	})

	t.Run("openai sdk style path", func(t *testing.T) {
		apiKey := createAPIKey(t, token, "sdk-path-key")
		resp := doRequest(t, "POST", "/v1/proxy/proxy-happy/chat/completions", map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello via SDK path"},
			},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["object"] != "chat.completion" {
			t.Errorf("expected raw chat.completion response, got %v", result["object"])
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
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		if got := resp.Header.Get("X-Sheeld-Status"); got != "rejected" {
			t.Errorf("expected X-Sheeld-Status 'rejected', got %q", got)
		}
		var result map[string]map[string]interface{}
		decodeBody(t, resp, &result)
		if result["error"]["type"] != "guardrail_rejection" {
			t.Errorf("expected error type 'guardrail_rejection', got %v", result["error"]["type"])
		}
		if result["error"]["code"] != "input_rejected" {
			t.Errorf("expected error code 'input_rejected', got %v", result["error"]["code"])
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
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		var result map[string]map[string]interface{}
		decodeBody(t, resp, &result)
		if result["error"]["type"] != "guardrail_rejection" {
			t.Errorf("expected error type 'guardrail_rejection', got %v", result["error"]["type"])
		}
		if result["error"]["code"] != "output_rejected" {
			t.Errorf("expected error code 'output_rejected', got %v", result["error"]["code"])
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
		if result["object"] != "chat.completion" {
			t.Errorf("expected raw chat.completion response, got %v", result["object"])
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

	// The audit writer batches asynchronously (1s flush interval); wait for
	// the proxy entries above to land before querying.
	time.Sleep(1500 * time.Millisecond)

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

func TestProxyWebhookGuard(t *testing.T) {
	token := registerUser(t, "Webhook Test Org", "webhook@example.com", "strongpassword123")
	setMockLLMResponse("Hello! I'm a helpful assistant.")

	// Stub webhook endpoint: rejects input containing "bad", records the body.
	var lastReq struct {
		Input       string `json:"input"`
		Phase       string `json:"phase"`
		SourceRoute string `json:"source_route"`
	}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&lastReq)
		passed := !strings.Contains(lastReq.Input, "bad")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"passed":  passed,
			"message": "checked by stub",
		})
	}))
	defer stub.Close()

	sourceID := createSource(t, token, "Webhook Source", "webhook-source")
	gID := createGuardrail(t, token, map[string]interface{}{
		"name":       "Stub Webhook",
		"guard_type": "webhook",
		"phase":      "input",
		"config":     map[string]interface{}{"url": stub.URL},
		"enabled":    true,
	})
	attachGuardrail(t, token, gID, sourceID)
	apiKey := createAPIKey(t, token, "webhook-key")

	t.Run("pass", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/webhook-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "hello there"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		if lastReq.Phase != "input" || lastReq.SourceRoute != "webhook-source" {
			t.Errorf("expected call meta in webhook request, got %+v", lastReq)
		}
	})

	t.Run("reject", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/webhook-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "something bad"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		var result map[string]map[string]interface{}
		decodeBody(t, resp, &result)
		if result["error"]["type"] != "guardrail_rejection" {
			t.Errorf("expected guardrail_rejection, got %v", result["error"]["type"])
		}
	})
}

func TestTransformers(t *testing.T) {
	token := registerUser(t, "Transformer Org", "transformer@example.com", "strongpassword123")
	setMockLLMResponse("Hello! I'm a helpful assistant.")

	sourceID := createSource(t, token, "Transform Source", "transform-source")

	t.Run("create validates type against registry", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             "Bogus",
			"transformer_type": "does_not_exist",
		}, token)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()
	})

	t.Run("create rejects unknown phase", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             "BothPhase",
			"transformer_type": "test_replace",
			"phase":            "both",
			"config":           map[string]interface{}{"find": "x"},
		}, token)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()
	})

	// Create two transformers: a→b then b→c proves chain ordering.
	mkTransformer := func(name, find, replace string) string {
		resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             name,
			"transformer_type": "test_replace",
			"config":           map[string]interface{}{"find": find, "replace": replace},
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		return result["id"].(string)
	}
	t1 := mkTransformer("first", "alpha", "beta")
	t2 := mkTransformer("second", "beta", "gamma")

	// Attach both (append order t1, t2), then reorder to t2, t1 and back to
	// verify the reorder endpoint.
	for _, id := range []string{t1, t2} {
		resp := doRequest(t, "POST", "/v1/transformers/"+id+"/sources",
			map[string]interface{}{"source_id": sourceID}, token)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	t.Run("list by source is ordered", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/sources/"+sourceID+"/transformers", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var list []map[string]interface{}
		decodeBody(t, resp, &list)
		if len(list) != 2 || list[0]["name"] != "first" || list[1]["name"] != "second" {
			t.Fatalf("unexpected order: %v", list)
		}
	})

	t.Run("reorder replaces chain order", func(t *testing.T) {
		resp := doRequest(t, "PUT", "/v1/sources/"+sourceID+"/transformers",
			map[string]interface{}{"transformer_ids": []string{t2, t1}}, token)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		resp = doRequest(t, "GET", "/v1/sources/"+sourceID+"/transformers", nil, token)
		expectStatus(t, resp, http.StatusOK)
		var list []map[string]interface{}
		decodeBody(t, resp, &list)
		if list[0]["name"] != "second" || list[1]["name"] != "first" {
			t.Fatalf("reorder not applied: %v", list)
		}

		// restore chain order for the e2e case
		resp = doRequest(t, "PUT", "/v1/sources/"+sourceID+"/transformers",
			map[string]interface{}{"transformer_ids": []string{t1, t2}}, token)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("proxy applies chain sequentially before LLM", func(t *testing.T) {
		apiKey := createAPIKey(t, token, "transformer-key")
		resp := doRequest(t, "POST", "/v1/proxy/transform-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "say alpha"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		// a→b then b→c: only sequential position-ordered application
		// yields gamma at the mock LLM.
		lastReq := getMockLLMLastRequest()
		if len(lastReq.Messages) == 0 || lastReq.Messages[len(lastReq.Messages)-1].Content != "say gamma" {
			t.Errorf("expected LLM to receive 'say gamma', got %+v", lastReq.Messages)
		}
	})
}

// TestBuiltinTransformers exercises the shipped transformer types end to
// end: a regex_replace and a webhook transformer chained on one source,
// observed at the mock LLM.
func TestBuiltinTransformers(t *testing.T) {
	token := registerUser(t, "Builtin Transformer Org", "builtin-transformer@example.com", "strongpassword123")
	setMockLLMResponse("done")
	sourceID := createSource(t, token, "Builtin Transform Source", "builtin-transform")

	// User-hosted rewrite endpoint: replaces "42" with "XX" in every message.
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("webhook decode: %v", err)
		}
		for i := range req.Messages {
			req.Messages[i].Content = strings.ReplaceAll(req.Messages[i].Content, "42", "XX")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": req.Messages})
	}))
	defer hook.Close()

	mk := func(name, transformerType string, cfg map[string]interface{}) string {
		resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             name,
			"transformer_type": transformerType,
			"config":           cfg,
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		return result["id"].(string)
	}

	regexID := mk("mask-secret", "regex_replace", map[string]interface{}{
		"rules": []map[string]string{{"pattern": `\bsecret\b`, "replace": "[MASKED]"}},
	})
	hookID := mk("rewrite-hook", "webhook", map[string]interface{}{"url": hook.URL})

	for _, id := range []string{regexID, hookID} {
		resp := doRequest(t, "POST", "/v1/transformers/"+id+"/sources",
			map[string]interface{}{"source_id": sourceID}, token)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	t.Run("create validates config", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             "bad-regex",
			"transformer_type": "regex_replace",
			"config":           map[string]interface{}{"rules": []map[string]string{{"pattern": "[", "replace": ""}}},
		}, token)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()

		resp = doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             "bad-presidio",
			"transformer_type": "presidio",
			"config":           map[string]interface{}{"analyzer_url": "http://a:3000"},
		}, token)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()
	})

	t.Run("proxy applies regex_replace then webhook", func(t *testing.T) {
		apiKey := createAPIKey(t, token, "builtin-transformer-key")
		resp := doRequest(t, "POST", "/v1/proxy/builtin-transform", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "the secret is 42"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		lastReq := getMockLLMLastRequest()
		if len(lastReq.Messages) == 0 || lastReq.Messages[len(lastReq.Messages)-1].Content != "the [MASKED] is XX" {
			t.Errorf("expected LLM to receive 'the [MASKED] is XX', got %+v", lastReq.Messages)
		}
	})
}

// TestOutputTransformers proves an output-phase transformer rewrites the
// LLM response before it reaches the client.
func TestOutputTransformers(t *testing.T) {
	token := registerUser(t, "Output Transformer Org", "output-transformer@example.com", "strongpassword123")
	setMockLLMResponse("the secret is 42")
	sourceID := createSource(t, token, "Output Transform Source", "output-transform")

	resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
		"name":             "mask-output",
		"transformer_type": "regex_replace",
		"phase":            "output",
		"config": map[string]interface{}{
			"rules": []map[string]string{{"pattern": `\bsecret\b`, "replace": "[MASKED]"}},
		},
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	if created["phase"] != "output" {
		t.Fatalf("expected output phase, got %v", created["phase"])
	}

	resp = doRequest(t, "POST", "/v1/transformers/"+created["id"].(string)+"/sources",
		map[string]interface{}{"source_id": sourceID}, token)
	expectStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	apiKey := createAPIKey(t, token, "output-transformer-key")
	resp = doRequest(t, "POST", "/v1/proxy/output-transform", map[string]interface{}{
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, apiKey)
	expectStatus(t, resp, http.StatusOK)
	if got := resp.Header.Get("X-Sheeld-Status"); got != "pass" {
		t.Errorf("X-Sheeld-Status = %q, want pass", got)
	}
	// The proxy body is the OpenAI-compatible chat completion — the client
	// must receive the transformed text.
	var chatResp llm.ChatResponse
	decodeBody(t, resp, &chatResp)
	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content != "the [MASKED] is 42" {
		t.Errorf("client response not transformed: %+v", chatResp.Choices)
	}
}
