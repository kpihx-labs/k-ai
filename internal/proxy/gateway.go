package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kpihx-labs/k-ai/internal/alias"
	"github.com/kpihx-labs/k-ai/internal/resolver"
	"github.com/kpihx-labs/k-ai/internal/store"
)

type Gateway struct {
	store        *store.Store
	client       *http.Client
	stream       *http.Client
	modelCache   *resolver.ModelCache
}

func NewGateway(st *store.Store) *Gateway {
	return &Gateway{
		store: st,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		stream:     &http.Client{},
		modelCache: resolver.NewModelCache(),
	}
}

// ModelCache exposes the shared upstream model cache for warmup.
func (g *Gateway) ModelCache() *resolver.ModelCache {
	return g.modelCache
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages json.RawMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type modelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type modelsResponse struct {
	Object string        `json:"object"`
	Data   []modelObject `json:"data"`
}

func (g *Gateway) ListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	registry, err := g.store.BuildRegistry(ctx, g.modelCache)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	aliasEngine, err := g.store.BuildAliasEngine(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	data := g.collectCatalogModels(ctx, registry, aliasEngine)
	writeJSON(w, http.StatusOK, modelsResponse{Object: "list", Data: data})
}

func (g *Gateway) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	upstreamModel := req.Model
	providerID := ""

	aliasEngine, err := g.store.BuildAliasEngine(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if match, ok := aliasEngine.Resolve(req.Model); ok {
		upstreamModel = match.UpstreamModel
		providerID = match.ProviderID
	}

	registry, err := g.store.BuildRegistry(ctx, g.modelCache)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var resolution *resolver.Resolution
	if providerID != "" {
		resolution, err = registry.ResolveWithProvider(providerID, upstreamModel)
	} else {
		resolution, err = registry.ResolveModel(upstreamModel)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	payload["model"] = resolution.UpstreamModel
	sanitizePayload(payload, resolution.Provider.ID)
	upstreamBody, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	targetURL := strings.TrimSuffix(resolution.Provider.BaseURL, "/") + "/chat/completions"
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	if resolution.Provider.APIKey != "" {
		upReq.Header.Set("Authorization", "Bearer "+resolution.Provider.APIKey)
	}

	client := g.client
	if req.Stream {
		client = g.stream
	}

	resp, err := client.Do(upReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("upstream error: %v", err))
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if req.Stream {
		streamCopy(w, resp.Body)
		return
	}
	_, _ = io.Copy(w, resp.Body)
}

func streamCopy(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func (g *Gateway) MockChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Model == "" {
		req.Model = "mock-model"
	}
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		chunk := map[string]any{
			"id":      "chatcmpl-mock",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   req.Model,
			"choices": []any{
				map[string]any{
					"index": 0,
					"delta": map[string]any{
						"role":    "assistant",
						"content": fmt.Sprintf("mock stream for %s", req.Model),
					},
				},
			},
		}
		raw, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", raw)
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	resp := map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": fmt.Sprintf("mock response for model %s", req.Model),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (g *Gateway) MockListModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, modelsResponse{
		Object: "list",
		Data: []modelObject{{ID: "mock-model", Object: "model", Created: time.Now().Unix(), OwnedBy: "mock"}},
	})
}

// sanitizePayload caps max_tokens and removes unsupported params per provider.
func sanitizePayload(p map[string]any, providerID string) {
	capTokens(p, "max_tokens")
	capTokens(p, "max_completion_tokens")

	maxForProvider := maxTokensForProvider(providerID)
	if maxForProvider > 0 {
		for _, key := range []string{"max_tokens", "max_completion_tokens"} {
			if v, ok := p[key]; ok {
				if n, isNum := toInt(v); isNum && n > maxForProvider {
					p[key] = maxForProvider
				}
			}
		}
	}
}

func maxTokensForProvider(id string) int {
	switch id {
	case "venice":
		return 16384
	case "ollama-local":
		return 8192
	default:
		return 0
	}
}

func capTokens(p map[string]any, key string) {
	if v, ok := p[key]; ok {
		if n, isNum := toInt(v); isNum && n <= 0 {
			delete(p, key)
		} else if !isNum {
			_ = n
		}
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

func copyHeader(dst, src http.Header) {
	for k, vals := range src {
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"message": msg, "type": "k_ai_error"},
	})
}

// ValidateAPIKey implements auth.KeyValidator.
type KeyValidator struct {
	Store *store.Store
}

func (v KeyValidator) ValidateAPIKey(raw string) ([]string, bool) {
	row, err := v.Store.ValidateAPIKey(context.Background(), raw)
	if err != nil || row == nil {
		return nil, false
	}
	return row.Scopes, true
}

// ResolveForTest exposes routing for unit tests.
func ResolveForTest(aliasEngine *alias.Engine, registry *resolver.Registry, model string) (*resolver.Resolution, string, error) {
	upstream := model
	providerID := ""
	if match, ok := aliasEngine.Resolve(model); ok {
		upstream = match.UpstreamModel
		providerID = match.ProviderID
	}
	var res *resolver.Resolution
	var err error
	if providerID != "" {
		res, err = registry.ResolveWithProvider(providerID, upstream)
	} else {
		res, err = registry.ResolveModel(upstream)
	}
	return res, upstream, err
}
