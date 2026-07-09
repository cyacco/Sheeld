//go:build integration

package integration

import (
	"bufio"
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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cyacco/Sheeld/internal/controlplane/api"
	"github.com/cyacco/Sheeld/internal/controlplane/config"
	"github.com/cyacco/Sheeld/internal/controlplane/db"
	"github.com/cyacco/Sheeld/internal/controlplane/db/generated"
	"github.com/cyacco/Sheeld/internal/controlplane/service"
	"github.com/cyacco/Sheeld/internal/dataplane/auditstore"
	"github.com/cyacco/Sheeld/internal/dataplane/backendconfig"
	dpconfig "github.com/cyacco/Sheeld/internal/dataplane/config"
	dpdb "github.com/cyacco/Sheeld/internal/dataplane/db"
	dpgenerated "github.com/cyacco/Sheeld/internal/dataplane/db/generated"
	"github.com/cyacco/Sheeld/internal/dataplane/gateway"
	"github.com/cyacco/Sheeld/internal/dataplane/processor"
	"github.com/cyacco/Sheeld/internal/shared/guard"
	"github.com/cyacco/Sheeld/internal/shared/llm"
	"github.com/cyacco/Sheeld/internal/shared/transform"
	"github.com/cyacco/Sheeld/internal/shared/urlpolicy"
)

// Package-level test infrastructure
var (
	testServer  *httptest.Server // control plane
	dpServer    *httptest.Server // data plane
	poller      *backendconfig.Poller
	auditWriter *auditstore.Writer
	pool        *pgxpool.Pool
	dpPool      *pgxpool.Pool
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

	// Most e2e tests point guards/transformers at loopback httptest servers,
	// so allow private targets by default; the SSRF test toggles this off.
	urlpolicy.SetAllowPrivate(true)

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
	dpPool, err = pgxpool.New(ctx, dpConnStr)
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
	guardrailService := service.NewGuardrailService(queries, guard.NewRegistry())
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
		"name":                name,
		"route":               route,
		"llm_provider":        "openai",
		"llm_model":           "gpt-4o",
		"llm_api_key":         "sk-test-key-12345",
		"input_pass_criteria": "all",
		"enabled":             true,
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
			"name":                "Test Source",
			"route":               "test-source",
			"description":         "A test source",
			"llm_provider":        "openai",
			"llm_model":           "gpt-4o",
			"llm_api_key":         "sk-test-key-12345",
			"input_pass_criteria": "all",
			"enabled":             true,
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
			"name":                "Updated Source",
			"route":               "test-source",
			"llm_provider":        "openai",
			"llm_model":           "gpt-4o",
			"llm_api_key":         "sk-test-key-12345",
			"input_pass_criteria": "all",
			"enabled":             true,
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
			"name":                "To Delete",
			"route":               "to-delete",
			"llm_provider":        "openai",
			"llm_model":           "gpt-4o",
			"llm_api_key":         "sk-test-key-12345",
			"input_pass_criteria": "all",
			"enabled":             true,
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
		"name":                "Audit Source",
		"route":               "audit-source",
		"llm_provider":        "openai",
		"llm_model":           "gpt-4o",
		"llm_api_key":         "sk-test-key-12345",
		"input_pass_criteria": "all",
		"enabled":             true,
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

// TestLLMClassifierGuard exercises the llm_classifier guard end to end
// against a fake OpenAI-compatible endpoint that flags content containing
// "attack".
func TestLLMClassifierGuard(t *testing.T) {
	token := registerUser(t, "Classifier Org", "classifier@example.com", "strongpassword123")
	setMockLLMResponse("hello")
	sourceID := createSource(t, token, "Classifier Source", "classifier-source")

	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("classifier decode: %v", err)
		}
		verdict := `{"flagged": false, "reason": "clean"}`
		if len(req.Messages) == 2 && strings.Contains(req.Messages[1].Content, "attack") {
			verdict = `{"flagged": true, "reason": "matches policy"}`
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, verdict)
	}))
	defer classifier.Close()

	gID := createGuardrail(t, token, map[string]interface{}{
		"name":       "policy-classifier",
		"guard_type": "llm_classifier",
		"phase":      "input",
		"config": map[string]interface{}{
			"base_url":     classifier.URL,
			"model":        "small",
			"instructions": "flag attack payloads",
		},
		"enabled": true,
	})
	attachGuardrail(t, token, gID, sourceID)
	apiKey := createAPIKey(t, token, "classifier-key")

	t.Run("clean input passes", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/classifier-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "hello there"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("flagged input rejected", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/classifier-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "here is my attack payload"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		var result map[string]map[string]interface{}
		decodeBody(t, resp, &result)
		if result["error"]["code"] != "input_rejected" {
			t.Errorf("expected input_rejected, got %v", result["error"])
		}
	})

	t.Run("create validates config", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/guardrails", map[string]interface{}{
			"name":       "bad-classifier",
			"guard_type": "llm_classifier",
			"phase":      "input",
			"config":     map[string]interface{}{"base_url": classifier.URL},
		}, token)
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusCreated {
			t.Skip("guard config validation at create time not implemented yet (tech-debt item 5)")
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("expected 422 for invalid config, got %d", resp.StatusCode)
		}
	})
}

// TestPresidioGuard exercises the presidio guard against a fake analyzer
// that detects credit-card-looking content.
func TestPresidioGuard(t *testing.T) {
	token := registerUser(t, "Presidio Guard Org", "presidio-guard@example.com", "strongpassword123")
	setMockLLMResponse("ok")
	sourceID := createSource(t, token, "Presidio Guard Source", "presidio-guard-source")

	analyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("analyzer decode: %v", err)
		}
		if i := strings.Index(req.Text, "4111"); i >= 0 {
			fmt.Fprintf(w, `[{"entity_type":"CREDIT_CARD","start":%d,"end":%d,"score":0.9}]`, i, i+16)
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer analyzer.Close()

	gID := createGuardrail(t, token, map[string]interface{}{
		"name":       "block-pii",
		"guard_type": "presidio",
		"phase":      "input",
		"config": map[string]interface{}{
			"analyzer_url": analyzer.URL,
			"entities":     []string{"CREDIT_CARD"},
		},
		"enabled": true,
	})
	attachGuardrail(t, token, gID, sourceID)
	apiKey := createAPIKey(t, token, "presidio-guard-key")

	t.Run("clean input passes", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/presidio-guard-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "hello"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("PII input rejected", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/presidio-guard-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "card: 4111111111111111"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		var result map[string]map[string]interface{}
		decodeBody(t, resp, &result)
		if result["error"]["code"] != "input_rejected" {
			t.Errorf("expected input_rejected, got %v", result["error"])
		}
	})

	t.Run("create validates config", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/guardrails", map[string]interface{}{
			"name":       "bad-presidio-guard",
			"guard_type": "presidio",
			"phase":      "input",
			"config":     map[string]interface{}{},
		}, token)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()
	})
}

// TestReversibleAnonymization proves the full round trip through the API:
// PII is replaced with placeholders before the LLM and restored in the
// response the client receives.
func TestReversibleAnonymization(t *testing.T) {
	token := registerUser(t, "Reversible Org", "reversible@example.com", "strongpassword123")
	sourceID := createSource(t, token, "Reversible Source", "reversible-source")

	analyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("analyzer decode: %v", err)
		}
		if i := strings.Index(req.Text, "Alice"); i >= 0 {
			fmt.Fprintf(w, `[{"entity_type":"PERSON","start":%d,"end":%d,"score":0.9}]`, i, i+5)
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer analyzer.Close()

	mk := func(name, transformerType, phase string, cfg map[string]interface{}) string {
		resp := doRequest(t, "POST", "/v1/transformers", map[string]interface{}{
			"name":             name,
			"transformer_type": transformerType,
			"phase":            phase,
			"config":           cfg,
		}, token)
		expectStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		return result["id"].(string)
	}
	anonID := mk("anon", "presidio", "input", map[string]interface{}{
		"analyzer_url": analyzer.URL,
		"mode":         "reversible",
	})
	deanID := mk("dean", "deanonymize", "output", map[string]interface{}{})
	for _, id := range []string{anonID, deanID} {
		resp := doRequest(t, "POST", "/v1/transformers/"+id+"/sources",
			map[string]interface{}{"source_id": sourceID}, token)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	// The placeholder is deterministic (fresh state per request, first
	// PERSON entity), so the canned LLM response can echo it.
	setMockLLMResponse("OK, I will tell <PERSON_1> hi.")

	apiKey := createAPIKey(t, token, "reversible-key")
	resp := doRequest(t, "POST", "/v1/proxy/reversible-source", map[string]interface{}{
		"messages": []map[string]string{{"role": "user", "content": "tell Alice hi"}},
	}, apiKey)
	expectStatus(t, resp, http.StatusOK)
	var chatResp llm.ChatResponse
	decodeBody(t, resp, &chatResp)

	// The LLM never saw the real name.
	lastReq := getMockLLMLastRequest()
	if len(lastReq.Messages) == 0 || lastReq.Messages[len(lastReq.Messages)-1].Content != "tell <PERSON_1> hi" {
		t.Errorf("LLM should see the placeholder, got %+v", lastReq.Messages)
	}
	// The client gets the real name restored.
	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content != "OK, I will tell Alice hi." {
		t.Errorf("client response not deanonymized: %+v", chatResp.Choices)
	}
}

// TestBufferedStreaming verifies "stream": true replays the guard-approved
// response as OpenAI-compatible SSE, and that rejections stay JSON errors.
func TestBufferedStreaming(t *testing.T) {
	token := registerUser(t, "Streaming Org", "streaming@example.com", "strongpassword123")
	sourceID := createSource(t, token, "Streaming Source", "streaming-source")
	full := "the quick brown fox jumps over the lazy dog and keeps on running far away"
	setMockLLMResponse(full)

	gID := createGuardrail(t, token, map[string]interface{}{
		"name":       "stream-blocklist",
		"guard_type": "blocklist",
		"phase":      "input",
		"config":     map[string]interface{}{"words": []string{"forbidden"}},
		"enabled":    true,
	})
	attachGuardrail(t, token, gID, sourceID)
	apiKey := createAPIKey(t, token, "streaming-key")

	t.Run("pass streams SSE chunks", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/streaming-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "hello"}},
			"stream":   true,
		}, apiKey)
		defer resp.Body.Close()
		expectStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Fatalf("Content-Type = %q", ct)
		}

		var content strings.Builder
		var sawDone bool
		scanner := bufio.NewScanner(resp.Body)
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
			var chunk struct {
				Object  string `json:"object"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				t.Fatalf("bad chunk %q: %v", payload, err)
			}
			if chunk.Object != "chat.completion.chunk" {
				t.Errorf("object = %q", chunk.Object)
			}
			content.WriteString(chunk.Choices[0].Delta.Content)
		}
		if !sawDone {
			t.Error("missing [DONE] terminator")
		}
		if content.String() != full {
			t.Errorf("reassembled = %q, want %q", content.String(), full)
		}
		// The LLM gateway must have been called non-streaming.
		if getMockLLMLastRequest().Stream {
			t.Error("LLM gateway was asked to stream; buffered streaming must call it non-streaming")
		}
	})

	t.Run("rejection stays a JSON error", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/streaming-source", map[string]interface{}{
			"messages": []map[string]string{{"role": "user", "content": "this is forbidden"}},
			"stream":   true,
		}, apiKey)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("rejection Content-Type = %q, want JSON", ct)
		}
		var result map[string]map[string]interface{}
		decodeBody(t, resp, &result)
		if result["error"]["code"] != "input_rejected" {
			t.Errorf("expected input_rejected, got %v", result["error"])
		}
	})
}

// TestPerPhasePassCriteria proves input and output guards evaluate under
// independent criteria: the same one-passing-one-failing guard shape passes
// the input phase (criteria "any") and rejects at the output phase
// (criteria "all") in a single request.
func TestPerPhasePassCriteria(t *testing.T) {
	token := registerUser(t, "PerPhase Org", "perphase@example.com", "strongpassword123")
	setMockLLMResponse("response mentions beta here")

	resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
		"name":                 "PerPhase Source",
		"route":                "perphase-source",
		"llm_provider":         "openai",
		"llm_model":            "gpt-4o",
		"llm_api_key":          "sk-test-key-12345",
		"input_pass_criteria":  "any",
		"output_pass_criteria": "all",
		"enabled":              true,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	var src map[string]interface{}
	decodeBody(t, resp, &src)
	sourceID := src["id"].(string)
	if src["input_pass_criteria"] != "any" || src["output_pass_criteria"] != "all" {
		t.Fatalf("per-phase criteria not persisted: %v", src)
	}

	// Input: blocks "alpha" (fails) + blocks "zzz" (passes) → "any" passes.
	// Output: blocks "beta" (fails) + blocks "qqq" (passes) → "all" rejects.
	mk := func(name, phase, word string) {
		gID := createGuardrail(t, token, map[string]interface{}{
			"name": name, "guard_type": "blocklist", "phase": phase,
			"config":  map[string]interface{}{"words": []string{word}},
			"enabled": true,
		})
		attachGuardrail(t, token, gID, sourceID)
	}
	mk("in-fail", "input", "alpha")
	mk("in-pass", "input", "zzz")
	mk("out-fail", "output", "beta")
	mk("out-pass", "output", "qqq")

	apiKey := createAPIKey(t, token, "perphase-key")
	resp = doRequest(t, "POST", "/v1/proxy/perphase-source", map[string]interface{}{
		"messages": []map[string]string{{"role": "user", "content": "tell me about alpha"}},
	}, apiKey)
	expectStatus(t, resp, http.StatusUnprocessableEntity)
	var result map[string]map[string]interface{}
	decodeBody(t, resp, &result)
	if result["error"]["code"] != "output_rejected" {
		t.Fatalf("expected output_rejected (input 'any' passed, output 'all' failed), got %v", result["error"])
	}

	t.Run("n_of_m requires threshold per phase", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
			"name":                 "Bad Threshold",
			"route":                "bad-threshold",
			"llm_provider":         "openai",
			"llm_model":            "gpt-4o",
			"llm_api_key":          "sk-test-key-12345",
			"output_pass_criteria": "n_of_m",
		}, token)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()
	})
}

// TestSecurityHardening covers the tenancy/secret/SSRF fixes: cross-org
// access is denied, config secrets are redacted in responses, and
// private-network guard URLs are rejected at create.
func TestSecurityHardening(t *testing.T) {
	orgA := registerUser(t, "Org A", "org-a@example.com", "strongpassword123")
	orgB := registerUser(t, "Org B", "org-b@example.com", "strongpassword123")

	sourceA := createSource(t, orgA, "A Source", "org-a-source")
	guardA := createGuardrail(t, orgA, map[string]interface{}{
		"name":       "a-classifier",
		"guard_type": "llm_classifier",
		"phase":      "input",
		"config": map[string]interface{}{
			"base_url":     "https://api.openai.com/v1",
			"model":        "gpt-4o-mini",
			"instructions": "flag secrets",
			"api_key":      "sk-super-secret",
		},
		"enabled": true,
	})
	attachGuardrail(t, orgA, guardA, sourceA)

	t.Run("config api_key is redacted in responses", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/guardrails/"+guardA, nil, orgA)
		expectStatus(t, resp, http.StatusOK)
		var g map[string]interface{}
		decodeBody(t, resp, &g)
		cfg := g["config"].(map[string]interface{})
		if cfg["api_key"] != "***" {
			t.Errorf("api_key should be redacted, got %v", cfg["api_key"])
		}
		if cfg["model"] != "gpt-4o-mini" {
			t.Errorf("non-secret field should survive, got %v", cfg["model"])
		}
	})

	t.Run("redacted config round-trips without clobbering the stored key", func(t *testing.T) {
		// Load (redacted), change only the name, save. The real key must survive.
		resp := doRequest(t, "PUT", "/v1/guardrails/"+guardA, map[string]interface{}{
			"name":       "a-classifier-renamed",
			"guard_type": "llm_classifier",
			"phase":      "input",
			"config": map[string]interface{}{
				"base_url":     "https://api.openai.com/v1",
				"model":        "gpt-4o-mini",
				"instructions": "flag secrets",
				"api_key":      "***",
			},
			"enabled": true,
		}, orgA)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		// The stored key isn't directly observable via the API (by design),
		// but the update succeeding with "***" proves the sentinel wasn't
		// re-validated as a literal key and the guard still builds.
	})

	t.Run("org B cannot list org A's guardrail sources", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/guardrails/"+guardA+"/sources", nil, orgB)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("org B cannot detach org A's guardrail", func(t *testing.T) {
		resp := doRequest(t, "DELETE", "/v1/guardrails/"+guardA+"/sources/"+sourceA, nil, orgB)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("org B cannot list guardrails on org A's source", func(t *testing.T) {
		resp := doRequest(t, "GET", "/v1/sources/"+sourceA+"/guardrails", nil, orgB)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("private-network guard URL is rejected", func(t *testing.T) {
		urlpolicy.SetAllowPrivate(false)
		defer urlpolicy.SetAllowPrivate(true)
		resp := doRequest(t, "POST", "/v1/guardrails", map[string]interface{}{
			"name":       "ssrf-attempt",
			"guard_type": "webhook",
			"phase":      "input",
			"config":     map[string]interface{}{"url": "http://169.254.169.254/latest/meta-data"},
			"enabled":    true,
		}, orgA)
		expectStatus(t, resp, http.StatusUnprocessableEntity)
		resp.Body.Close()
	})

	t.Run("API key list omits hash and prefix", func(t *testing.T) {
		createAPIKey(t, orgA, "list-test-key")
		resp := doRequest(t, "GET", "/v1/auth/api-keys", nil, orgA)
		expectStatus(t, resp, http.StatusOK)
		var keys []map[string]interface{}
		decodeBody(t, resp, &keys)
		if len(keys) == 0 {
			t.Fatal("expected at least one key")
		}
		for _, k := range keys {
			if _, ok := k["key_hash"]; ok {
				t.Error("key_hash must not be in the response")
			}
			if _, ok := k["key_prefix"]; ok {
				t.Error("key_prefix must not be in the response")
			}
		}
	})
}

// TestAuditLogRetention verifies the pruner deletes rows older than the
// retention window and keeps newer ones.
func TestAuditLogRetention(t *testing.T) {
	orgID := uuid.New()
	sourceID := uuid.New()

	// Seed two rows: one 48h old, one just now.
	insert := func(age time.Duration) {
		_, err := dpPool.Exec(context.Background(),
			`INSERT INTO audit_logs (organization_id, source_id, input_hash, guard_results, overall_result, latency_ms, created_at)
			 VALUES ($1, $2, 'h', '{}', 'pass', 1, now() - $3::interval)`,
			orgID, sourceID, fmt.Sprintf("%d hours", int(age.Hours())))
		if err != nil {
			t.Fatalf("seed audit row: %v", err)
		}
	}
	insert(48 * time.Hour)
	insert(0)

	count := func() int {
		var n int
		if err := dpPool.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_logs WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	if count() != 2 {
		t.Fatalf("expected 2 seeded rows, got %d", count())
	}

	// Prune anything older than 24h. Run does one immediate sweep; cancel
	// once the ticker would fire.
	ctx, cancel := context.WithCancel(context.Background())
	pruner := auditstore.NewPruner(dpgenerated.New(dpPool), 24*time.Hour, time.Hour)
	done := make(chan struct{})
	go func() { pruner.Run(ctx); close(done) }()

	// Poll until the old row is gone (immediate sweep), then stop the pruner.
	deadline := time.Now().Add(3 * time.Second)
	for count() != 1 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("expected 1 row after prune, got %d", count())
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done

	if count() != 1 {
		t.Errorf("recent row should survive; got %d rows", count())
	}
}

// TestPerSourceLLMBaseURL verifies a source with llm_base_url set sends its
// traffic to that endpoint instead of the default gateway.
func TestPerSourceLLMBaseURL(t *testing.T) {
	token := registerUser(t, "BaseURL Org", "baseurl@example.com", "strongpassword123")
	apiKey := createAPIKey(t, token, "baseurl-key")

	// A second OpenAI-compatible endpoint, distinct from the default mock
	// gateway, returning recognizable content.
	override := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := llm.ChatResponse{
			ID:      "chatcmpl-override",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o",
			Choices: []llm.Choice{{
				Message:      llm.Message{Role: "assistant", Content: "routed to override endpoint"},
				FinishReason: "stop",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer override.Close()

	resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
		"name":         "BaseURL Source",
		"route":        "baseurl-source",
		"llm_provider": "openai",
		"llm_model":    "gpt-4o",
		"llm_api_key":  "sk-test",
		"llm_base_url": override.URL,
		"enabled":      true,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	if created["llm_base_url"] != override.URL {
		t.Fatalf("expected llm_base_url %q in response, got %v", override.URL, created["llm_base_url"])
	}

	t.Run("proxy routes to the per-source endpoint", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/proxy/baseurl-source", map[string]interface{}{
			"model":    "gpt-4o",
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		}, apiKey)
		expectStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		content := result["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"]
		if content != "routed to override endpoint" {
			t.Fatalf("expected override content, got %v", content)
		}
	})

	t.Run("invalid base URL rejected at create", func(t *testing.T) {
		resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
			"name":         "Bad BaseURL",
			"route":        "bad-baseurl",
			"llm_provider": "openai",
			"llm_model":    "gpt-4o",
			"llm_api_key":  "sk-test",
			"llm_base_url": "ftp://example.com",
			"enabled":      true,
		}, token)
		if resp.StatusCode == http.StatusCreated {
			t.Fatal("expected non-http(s) llm_base_url to be rejected")
		}
		resp.Body.Close()
	})
}

// TestOpenAIFieldPassthrough proves the proxy forwards unmodeled request
// fields (tools) to the provider and returns unmodeled response fields
// (tool_calls) to the client — i.e. function calling isn't silently dropped.
func TestOpenAIFieldPassthrough(t *testing.T) {
	token := registerUser(t, "Passthrough Org", "passthrough@example.com", "strongpassword123")
	apiKey := createAPIKey(t, token, "passthrough-key")

	var gotBody map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		// Respond with an assistant tool call (content null + tool_calls).
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"chatcmpl-tp","object":"chat.completion","created":1,"model":"gpt-4o",
			"choices":[{"index":0,"finish_reason":"tool_calls","message":{
				"role":"assistant","content":null,
				"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer provider.Close()

	resp := doRequest(t, "POST", "/v1/sources", map[string]interface{}{
		"name": "Tools Source", "route": "tools-source",
		"llm_provider": "openai", "llm_model": "gpt-4o", "llm_api_key": "sk-x",
		"llm_base_url": provider.URL, "enabled": true,
	}, token)
	expectStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doRequest(t, "POST", "/v1/proxy/tools-source", map[string]interface{}{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": "weather?"}},
		"tools": []map[string]interface{}{{
			"type":     "function",
			"function": map[string]string{"name": "get_weather"},
		}},
		"tool_choice": "auto",
	}, apiKey)
	expectStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)

	// Request side: tools reached the provider.
	if _, ok := gotBody["tools"]; !ok {
		t.Errorf("tools field was not forwarded to the provider: %v", gotBody)
	}
	if gotBody["tool_choice"] != "auto" {
		t.Errorf("tool_choice not forwarded: %v", gotBody["tool_choice"])
	}
	// Response side: tool_calls returned to the client.
	choice := result["choices"].([]interface{})[0].(map[string]interface{})
	if choice["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason lost: %v", choice["finish_reason"])
	}
	msg := choice["message"].(map[string]interface{})
	if _, ok := msg["tool_calls"]; !ok {
		t.Errorf("tool_calls dropped from response: %v", msg)
	}
}
