package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultURL = "http://127.0.0.1:8080"

// ANSI colors
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
)

var client = &http.Client{Timeout: 15 * time.Second}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func baseURL() string { return strings.TrimRight(env("K_AI_URL", defaultURL), "/") }

func adminToken() string { return os.Getenv("K_AI_ADMIN_TOKEN") }

func apiKey() string { return os.Getenv("K_AI_API_KEY") }

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, red+"error: "+reset+format+"\n", args...)
	os.Exit(1)
}

func requireAdminToken() string {
	t := adminToken()
	if t == "" {
		die("K_AI_ADMIN_TOKEN is not set")
	}
	return t
}

func requireAPIKey() string {
	k := apiKey()
	if k == "" {
		die("K_AI_API_KEY is not set")
	}
	return k
}

// doRequest performs an HTTP request and returns the body bytes.
// On non-2xx it prints the error and exits.
func doRequest(method, url string, body []byte, headers map[string]string) []byte {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		die("building request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		die("request failed: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		die("reading response: %v", err)
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &errResp) == nil && errResp.Error != "" {
			die("HTTP %d — %s", resp.StatusCode, errResp.Error)
		}
		die("HTTP %d — %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data
}

func adminReq(method, path string, body []byte) []byte {
	token := requireAdminToken()
	return doRequest(method, baseURL()+path, body, map[string]string{
		"X-Admin-Token": token,
	})
}

func apiReq(method, path string, body []byte) []byte {
	key := requireAPIKey()
	return doRequest(method, baseURL()+path, body, map[string]string{
		"Authorization": "Bearer " + key,
	})
}

func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

// --- Commands ---

func cmdHealth() {
	data := doRequest("GET", baseURL()+"/health", nil, nil)
	var h struct{ Status string }
	_ = json.Unmarshal(data, &h)
	if h.Status == "ok" {
		fmt.Printf(green+bold+"✓"+reset+" %s is "+green+"healthy"+reset+"\n", baseURL())
	} else {
		fmt.Printf(red+bold+"✗"+reset+" %s returned: %s\n", baseURL(), string(data))
		os.Exit(1)
	}

	// Also check admin endpoint if token is available
	if t := adminToken(); t != "" {
		adata := doRequest("GET", baseURL()+"/admin/api/v1/health", nil, map[string]string{
			"X-Admin-Token": t,
		})
		var ah struct{ Status string }
		_ = json.Unmarshal(adata, &ah)
		if ah.Status == "ok" {
			fmt.Printf(green+bold+"✓"+reset+" admin API is "+green+"healthy"+reset+"\n")
		}
	} else {
		fmt.Printf(dim+"  (set K_AI_ADMIN_TOKEN to also check admin API)"+reset+"\n")
	}
}

// --- Providers ---

func cmdProvidersList() {
	data := adminReq("GET", "/admin/api/v1/providers", nil)
	var resp struct {
		Providers []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			BaseURL  string `json:"base_url"`
			APIKey   string `json:"api_key"`
			Enabled  bool   `json:"enabled"`
			Priority int    `json:"priority"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		die("parsing response: %v", err)
	}

	if len(resp.Providers) == 0 {
		fmt.Println(dim + "No providers configured." + reset)
		return
	}

	fmt.Printf(bold+"%-18s %-12s %-8s %-4s %s"+reset+"\n", "ID", "NAME", "ENABLED", "PRI", "BASE URL")
	fmt.Println(dim + strings.Repeat("─", 78) + reset)
	for _, p := range resp.Providers {
		status := green + "yes" + reset
		if !p.Enabled {
			status = red + "no " + reset
		}
		fmt.Printf("%-18s %-12s %-18s %-4d %s\n", p.ID, p.Name, status, p.Priority, dim+p.BaseURL+reset)
	}
	fmt.Printf("\n"+dim+"%d provider(s)"+reset+"\n", len(resp.Providers))
}

func cmdProvidersGet(id string) {
	data := adminReq("GET", "/admin/api/v1/providers/"+id, nil)
	fmt.Println(prettyJSON(data))
}

func cmdProvidersDelete(id string) {
	adminReq("DELETE", "/admin/api/v1/providers/"+id, nil)
	fmt.Printf(green+"✓"+reset+" Provider "+bold+"%s"+reset+" deleted\n", id)
}

// --- Aliases ---

func cmdAliasesList() {
	data := adminReq("GET", "/admin/api/v1/aliases", nil)
	var resp struct {
		Aliases []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			MatchType  string `json:"match_type"`
			Pattern    string `json:"pattern"`
			Rewrite    string `json:"rewrite"`
			ProviderID string `json:"provider_id"`
			Priority   int    `json:"priority"`
			Enabled    bool   `json:"enabled"`
		} `json:"aliases"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		die("parsing response: %v", err)
	}

	if len(resp.Aliases) == 0 {
		fmt.Println(dim + "No aliases configured." + reset)
		return
	}

	fmt.Printf(bold+"%-16s %-10s %-24s %-16s %-7s"+reset+"\n", "ID", "TYPE", "PATTERN", "PROVIDER", "ENABLED")
	fmt.Println(dim + strings.Repeat("─", 78) + reset)
	for _, a := range resp.Aliases {
		status := green + "yes" + reset
		if !a.Enabled {
			status = red + "no " + reset
		}
		rewrite := ""
		if a.Rewrite != "" {
			rewrite = dim + " → " + a.Rewrite + reset
		}
		fmt.Printf("%-16s %-10s %-24s %-16s %-17s%s\n", a.ID, a.MatchType, a.Pattern, a.ProviderID, status, rewrite)
	}
	fmt.Printf("\n"+dim+"%d alias(es)"+reset+"\n", len(resp.Aliases))
}

func cmdAliasesDelete(id string) {
	adminReq("DELETE", "/admin/api/v1/aliases/"+id, nil)
	fmt.Printf(green+"✓"+reset+" Alias "+bold+"%s"+reset+" deleted\n", id)
}

// --- API Keys ---

func cmdKeysList() {
	data := adminReq("GET", "/admin/api/v1/api-keys", nil)
	var resp struct {
		Keys []struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Scopes    []string `json:"scopes"`
			Enabled   bool     `json:"enabled"`
			CreatedAt string   `json:"created_at"`
		} `json:"api_keys"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		die("parsing response: %v", err)
	}

	if len(resp.Keys) == 0 {
		fmt.Println(dim + "No API keys." + reset)
		return
	}

	fmt.Printf(bold+"%-36s %-14s %-20s %-7s"+reset+"\n", "ID", "NAME", "SCOPES", "ENABLED")
	fmt.Println(dim + strings.Repeat("─", 80) + reset)
	for _, k := range resp.Keys {
		status := green + "yes" + reset
		if !k.Enabled {
			status = red + "no " + reset
		}
		scopes := strings.Join(k.Scopes, ", ")
		fmt.Printf("%-36s %-14s %-20s %-17s\n", k.ID, k.Name, scopes, status)
	}
	fmt.Printf("\n"+dim+"%d key(s)"+reset+"\n", len(resp.Keys))
}

func cmdKeysCreate(name string) {
	body, _ := json.Marshal(map[string]any{
		"name":   name,
		"scopes": []string{"chat", "models"},
	})
	data := adminReq("POST", "/admin/api/v1/api-keys", body)

	var key struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(data, &key); err != nil {
		die("parsing response: %v", err)
	}

	fmt.Printf(green+"✓"+reset+" API key created\n\n")
	fmt.Printf("  "+bold+"Name:"+reset+"  %s\n", key.Name)
	fmt.Printf("  "+bold+"ID:"+reset+"    %s\n", key.ID)
	fmt.Printf("  "+bold+"Key:"+reset+"   "+yellow+"%s"+reset+"\n", key.Key)
	fmt.Printf("\n" + dim + "  Save this key — it won't be shown again." + reset + "\n")
}

func cmdKeysDelete(id string) {
	adminReq("DELETE", "/admin/api/v1/api-keys/"+id, nil)
	fmt.Printf(green+"✓"+reset+" API key "+bold+"%s"+reset+" deleted\n", id)
}

// --- Users (admin) ---

func cmdUsersList() {
	data := adminReq("GET", "/admin/api/v1/users", nil)
	var resp struct {
		Users []struct {
			ID        string `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			Role      string `json:"role"`
			Enabled   bool   `json:"enabled"`
			CreatedAt string `json:"created_at"`
		} `json:"users"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		die("parsing response: %v", err)
	}

	if len(resp.Users) == 0 {
		fmt.Println(dim + "No users." + reset)
		return
	}

	fmt.Printf(bold+"%-38s %-16s %-22s %-8s %-7s"+reset+"\n", "ID", "USERNAME", "EMAIL", "ROLE", "ENABLED")
	fmt.Println(dim + strings.Repeat("─", 95) + reset)
	for _, u := range resp.Users {
		status := green + "yes" + reset
		if !u.Enabled {
			status = red + "no " + reset
		}
		email := u.Email
		if email == "" {
			email = dim + "—" + reset
		}
		fmt.Printf("%-38s %-16s %-22s %-8s %-17s\n", u.ID, u.Username, email, u.Role, status)
	}
	fmt.Printf("\n"+dim+"%d user(s)"+reset+"\n", len(resp.Users))
}

func cmdUsersGet(id string) {
	data := adminReq("GET", "/admin/api/v1/users/"+id, nil)
	fmt.Println(prettyJSON(data))
}

func cmdUsersUpdate(id, field, value string) {
	body, _ := json.Marshal(map[string]string{field: value})
	data := adminReq("PUT", "/admin/api/v1/users/"+id, body)

	var u struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.Unmarshal(data, &u); err != nil {
		fmt.Println(prettyJSON(data))
		return
	}
	fmt.Printf(green+"✓"+reset+" User "+bold+"%s"+reset+" updated (%s=%s)\n", u.Username, field, value)
}

func cmdUsersDelete(id string) {
	adminReq("DELETE", "/admin/api/v1/users/"+id, nil)
	fmt.Printf(green+"✓"+reset+" User "+bold+"%s"+reset+" deleted\n", id)
}

// --- Auth (client) ---

func cmdAuthRegister(username, password, email string) {
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	if email != "" {
		payload["email"] = email
	}
	body, _ := json.Marshal(payload)
	data := doRequest("POST", baseURL()+"/auth/register", body, nil)

	var resp struct {
		Token string `json:"token"`
		User  struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Println(prettyJSON(data))
		return
	}

	fmt.Printf(green+"✓"+reset+" Registered as "+bold+"%s"+reset+" (role: %s)\n\n", resp.User.Username, resp.User.Role)
	fmt.Printf("  "+bold+"User ID:"+reset+"   %s\n", resp.User.ID)
	fmt.Printf("  "+bold+"Token:"+reset+"     "+yellow+"%s"+reset+"\n", resp.Token)
	if resp.APIKey != "" {
		fmt.Printf("  "+bold+"API Key:"+reset+"   "+yellow+"%s"+reset+"\n", resp.APIKey)
	}
	fmt.Printf("\n" + dim + "  Save these credentials — they won't be shown again." + reset + "\n")
}

func cmdAuthLogin(username, password string) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	data := doRequest("POST", baseURL()+"/auth/login", body, nil)

	var resp struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Println(prettyJSON(data))
		return
	}

	fmt.Printf(green+"✓"+reset+" Logged in as "+bold+"%s"+reset+" (role: %s)\n\n", resp.User.Username, resp.User.Role)
	fmt.Printf("  "+bold+"Token:"+reset+"  "+yellow+"%s"+reset+"\n", resp.Token)
}

func cmdAuthMe(token string) {
	data := doRequest("GET", baseURL()+"/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})
	fmt.Println(prettyJSON(data))
}

// --- Models ---

func cmdModels() {
	data := apiReq("GET", "/v1/models", nil)
	var resp struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		// Fallback: print raw JSON
		fmt.Println(prettyJSON(data))
		return
	}

	if len(resp.Data) == 0 {
		fmt.Println(dim + "No models available." + reset)
		return
	}

	fmt.Printf(bold+"%-40s %s"+reset+"\n", "MODEL", "OWNED BY")
	fmt.Println(dim + strings.Repeat("─", 60) + reset)
	for _, m := range resp.Data {
		fmt.Printf("%-40s %s\n", m.ID, dim+m.OwnedBy+reset)
	}
	fmt.Printf("\n"+dim+"%d model(s)"+reset+"\n", len(resp.Data))
}

// --- Chat ---

func cmdChat(model, message string) {
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": message},
		},
		"stream": false,
	})

	fmt.Printf(dim+"→ %s @ %s"+reset+"\n\n", model, baseURL())
	data := apiReq("POST", "/v1/chat/completions", body)

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Println(prettyJSON(data))
		return
	}

	if len(resp.Choices) > 0 {
		fmt.Println(resp.Choices[0].Message.Content)
		fmt.Printf("\n"+dim+"[tokens: %d prompt + %d completion = %d total]"+reset+"\n",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	} else {
		fmt.Println(dim + "No response choices returned." + reset)
	}
}

// --- Main ---

func usage() {
	prog := "k-ai-admin"
	if len(os.Args) > 0 {
		parts := strings.Split(os.Args[0], "/")
		prog = parts[len(parts)-1]
	}
	fmt.Printf(bold+"k-ai"+reset+" — sovereign LLM gateway admin CLI\n\n")
	fmt.Printf(bold+"Usage:"+reset+"\n")
	fmt.Printf("  %s "+cyan+"health"+reset+"                   Check server health\n", prog)
	fmt.Printf("  %s "+cyan+"providers list"+reset+"            List all providers\n", prog)
	fmt.Printf("  %s "+cyan+"providers get"+reset+" ID          Get a provider\n", prog)
	fmt.Printf("  %s "+cyan+"providers delete"+reset+" ID       Delete a provider\n", prog)
	fmt.Printf("  %s "+cyan+"aliases list"+reset+"              List all aliases\n", prog)
	fmt.Printf("  %s "+cyan+"aliases delete"+reset+" ID         Delete an alias\n", prog)
	fmt.Printf("  %s "+cyan+"keys list"+reset+"                 List API keys\n", prog)
	fmt.Printf("  %s "+cyan+"keys create"+reset+" NAME          Create an API key\n", prog)
	fmt.Printf("  %s "+cyan+"keys delete"+reset+" ID            Delete an API key\n", prog)
	fmt.Printf("  %s "+cyan+"users list"+reset+"                List users\n", prog)
	fmt.Printf("  %s "+cyan+"users get"+reset+" ID              Get user details\n", prog)
	fmt.Printf("  %s "+cyan+"users update"+reset+" ID F V       Update user field (role, enabled)\n", prog)
	fmt.Printf("  %s "+cyan+"users delete"+reset+" ID           Delete a user\n", prog)
	fmt.Printf("  %s "+cyan+"auth register"+reset+" USER PASS [EMAIL]  Register\n", prog)
	fmt.Printf("  %s "+cyan+"auth login"+reset+" USER PASS      Login and get JWT\n", prog)
	fmt.Printf("  %s "+cyan+"auth me"+reset+" TOKEN             Get current user info\n", prog)
	fmt.Printf("  %s "+cyan+"models"+reset+"                    List available models\n", prog)
	fmt.Printf("  %s "+cyan+"chat"+reset+" MODEL MESSAGE        Quick chat test\n", prog)
	fmt.Printf("\n"+bold+"Environment:"+reset+"\n")
	fmt.Printf("  "+yellow+"K_AI_URL"+reset+"          Server URL (default: %s)\n", defaultURL)
	fmt.Printf("  "+yellow+"K_AI_ADMIN_TOKEN"+reset+"  Admin token (for providers/aliases/keys)\n")
	fmt.Printf("  "+yellow+"K_AI_API_KEY"+reset+"      API key (for models/chat)\n")
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		usage()

	case "health":
		cmdHealth()

	case "providers":
		if len(rest) == 0 {
			die("usage: providers <list|get|delete> [ID]")
		}
		switch rest[0] {
		case "list", "ls":
			cmdProvidersList()
		case "get":
			if len(rest) < 2 {
				die("usage: providers get ID")
			}
			cmdProvidersGet(rest[1])
		case "delete", "rm":
			if len(rest) < 2 {
				die("usage: providers delete ID")
			}
			cmdProvidersDelete(rest[1])
		default:
			die("unknown providers subcommand: %s (use list, get, delete)", rest[0])
		}

	case "aliases":
		if len(rest) == 0 {
			die("usage: aliases <list|delete> [ID]")
		}
		switch rest[0] {
		case "list", "ls":
			cmdAliasesList()
		case "delete", "rm":
			if len(rest) < 2 {
				die("usage: aliases delete ID")
			}
			cmdAliasesDelete(rest[1])
		default:
			die("unknown aliases subcommand: %s (use list, delete)", rest[0])
		}

	case "keys":
		if len(rest) == 0 {
			die("usage: keys <list|create|delete> [NAME|ID]")
		}
		switch rest[0] {
		case "list", "ls":
			cmdKeysList()
		case "create":
			if len(rest) < 2 {
				die("usage: keys create NAME")
			}
			cmdKeysCreate(rest[1])
		case "delete", "rm":
			if len(rest) < 2 {
				die("usage: keys delete ID")
			}
			cmdKeysDelete(rest[1])
		default:
			die("unknown keys subcommand: %s (use list, create, delete)", rest[0])
		}

	case "users":
		if len(rest) == 0 {
			die("usage: users <list|get|update|delete> [ID] [FIELD VALUE]")
		}
		switch rest[0] {
		case "list", "ls":
			cmdUsersList()
		case "get":
			if len(rest) < 2 {
				die("usage: users get ID")
			}
			cmdUsersGet(rest[1])
		case "update":
			if len(rest) < 4 {
				die("usage: users update ID FIELD VALUE (e.g. users update UUID role admin)")
			}
			cmdUsersUpdate(rest[1], rest[2], rest[3])
		case "delete", "rm":
			if len(rest) < 2 {
				die("usage: users delete ID")
			}
			cmdUsersDelete(rest[1])
		default:
			die("unknown users subcommand: %s (use list, get, update, delete)", rest[0])
		}

	case "auth":
		if len(rest) == 0 {
			die("usage: auth <register|login|me> ...")
		}
		switch rest[0] {
		case "register":
			if len(rest) < 3 {
				die("usage: auth register USERNAME PASSWORD [EMAIL]")
			}
			email := ""
			if len(rest) >= 4 {
				email = rest[3]
			}
			cmdAuthRegister(rest[1], rest[2], email)
		case "login":
			if len(rest) < 3 {
				die("usage: auth login USERNAME PASSWORD")
			}
			cmdAuthLogin(rest[1], rest[2])
		case "me":
			if len(rest) < 2 {
				die("usage: auth me TOKEN")
			}
			cmdAuthMe(rest[1])
		default:
			die("unknown auth subcommand: %s (use register, login, me)", rest[0])
		}

	case "models":
		cmdModels()

	case "chat":
		if len(rest) < 2 {
			die("usage: chat MODEL MESSAGE")
		}
		model := rest[0]
		message := strings.Join(rest[1:], " ")
		cmdChat(model, message)

	default:
		die("unknown command: %s\nRun with --help for usage.", cmd)
	}
}
