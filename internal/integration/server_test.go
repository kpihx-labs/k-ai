package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kpihx-labs/k-ai/internal/config"
	"github.com/kpihx-labs/k-ai/internal/server"
	"github.com/kpihx-labs/k-ai/internal/store"
)

func setupTestServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := &config.Config{
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 0, AdminToken: "test-admin"},
		Database: config.DatabaseConfig{Path: dbPath},
		Providers: []config.ProviderConfig{
			{ID: "mock", Name: "Mock", BaseURL: "http://127.0.0.1:0/mock/v1", Enabled: true, Models: []config.ModelRule{
				{MatchType: config.MatchExact, Pattern: "mock-model"},
			}},
		},
		Aliases: []config.AliasRuleConfig{
			{ID: "alias1", MatchType: config.MatchRegex, Pattern: `^local-(?P<upstream>.+)$`, Rewrite: "${upstream}", ProviderID: "mock", Priority: 10, Enabled: true},
		},
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BootstrapFromConfig(cfg); err != nil {
		t.Fatal(err)
	}
	srv := server.New(cfg, st)
	ts := httptest.NewServer(srv.Handler())
	cfg.Providers[0].BaseURL = ts.URL + "/mock/v1"
	_ = st.UpsertProvider(t.Context(), store.ProviderRow{
		ID: "mock", Name: "Mock", BaseURL: ts.URL + "/mock/v1", Enabled: true,
		Models: []config.ModelRule{{MatchType: config.MatchExact, Pattern: "mock-model"}},
	})

	created, err := st.CreateAPIKey(t.Context(), "integration", []string{"chat", "models"})
	if err != nil {
		t.Fatal(err)
	}
	apiKey := created.Key
	return ts, st, apiKey
}

func TestIntegrationHealth(t *testing.T) {
	ts, st, _ := setupTestServer(t)
	defer ts.Close()
	defer st.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestIntegrationChatViaAlias(t *testing.T) {
	ts, st, apiKey := setupTestServer(t)
	defer ts.Close()
	defer st.Close()

	body := map[string]any{
		"model": "local-mock-model",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, string(b))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["object"] != "chat.completion" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestIntegrationAdminCRUD(t *testing.T) {
	ts, st, _ := setupTestServer(t)
	defer ts.Close()
	defer st.Close()

	createBody := `{"id":"extra","name":"Extra","base_url":"` + ts.URL + `/mock/v1","enabled":true,"models":[{"match_type":"exact","pattern":"x"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/api/v1/providers", bytes.NewBufferString(createBody))
	req.Header.Set("X-Admin-Token", "test-admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create provider status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/admin/api/v1/providers", nil)
	req.Header.Set("X-Admin-Token", "test-admin")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list providers status %d", resp.StatusCode)
	}
}

func TestIntegrationDashboard(t *testing.T) {
	ts, st, _ := setupTestServer(t)
	defer ts.Close()
	defer st.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("k-ai")) {
		t.Fatalf("dashboard body missing title")
	}
}

func TestIntegrationStreamMock(t *testing.T) {
	ts, st, apiKey := setupTestServer(t)
	defer ts.Close()
	defer st.Close()

	body := map[string]any{
		"model":  "mock-model",
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, string(b))
	}
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("chat.completion.chunk")) {
		t.Fatalf("expected stream chunk, got %s", string(b))
	}
}
