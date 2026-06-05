package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kpihx-labs/k-ai/internal/alias"
	"github.com/kpihx-labs/k-ai/internal/config"
	"github.com/kpihx-labs/k-ai/internal/resolver"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type ProviderRow struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	BaseURL  string            `json:"base_url"`
	APIKey   string            `json:"api_key,omitempty"`
	Enabled  bool              `json:"enabled"`
	Priority int               `json:"priority"`
	Models  []config.ModelRule `json:"models"`
}

type AliasRow struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	MatchType  config.MatchType  `json:"match_type"`
	Pattern    string            `json:"pattern"`
	Rewrite    string            `json:"rewrite"`
	ProviderID string            `json:"provider_id"`
	Priority   int               `json:"priority"`
	Prefix     string            `json:"prefix"`
	Suffix     string            `json:"suffix"`
	Enabled    bool              `json:"enabled"`
}

type APIKeyRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key,omitempty"`
	Scopes    []string  `json:"scopes"`
	Enabled   bool      `json:"enabled"`
	UserID    string    `json:"user_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type UserRow struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Email        string    `json:"email,omitempty"`
	Role         string    `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS providers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			models_json TEXT NOT NULL DEFAULT '[]',
			priority INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS alias_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			match_type TEXT NOT NULL,
			pattern TEXT NOT NULL,
			rewrite TEXT NOT NULL DEFAULT '',
			provider_id TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			prefix TEXT NOT NULL DEFAULT '',
			suffix TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY(provider_id) REFERENCES providers(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			scopes_json TEXT NOT NULL DEFAULT '["chat"]',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	// Migration: add priority column to providers if missing
	s.db.Exec(`ALTER TABLE providers ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`)

	// Migration: users table
	s.db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'user',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)

	// Migration: link api_keys to users (no FK constraint — empty string means unlinked)
	s.db.Exec(`ALTER TABLE api_keys ADD COLUMN user_id TEXT NOT NULL DEFAULT ''`)

	return nil
}

func (s *Store) BootstrapFromConfig(cfg *config.Config) error {
	ctx := context.Background()
	// Temporarily disable FK constraints during bootstrap upsert cycle
	s.db.Exec(`PRAGMA foreign_keys = OFF`)
	defer s.db.Exec(`PRAGMA foreign_keys = ON`)
	for _, p := range cfg.Providers {
		if err := s.UpsertProvider(ctx, ProviderRow{
			ID: p.ID, Name: p.Name, BaseURL: p.BaseURL, APIKey: p.APIKey,
			Enabled: p.Enabled, Models: p.Models, Priority: p.Priority,
		}); err != nil {
			return err
		}
	}
	for _, a := range cfg.Aliases {
		if err := s.UpsertAlias(ctx, AliasRow{
			ID: a.ID, Name: a.Name, MatchType: a.MatchType, Pattern: a.Pattern,
			Rewrite: a.Rewrite, ProviderID: a.ProviderID, Priority: a.Priority,
			Prefix: a.Prefix, Suffix: a.Suffix, Enabled: a.Enabled,
		}); err != nil {
			return err
		}
	}
	for _, k := range cfg.APIKeys {
		if _, err := s.CreateAPIKey(ctx, k.Name, k.Scopes); err != nil {
			return err
		}
	}
	keyCount, err := s.countAPIKeys(ctx)
	if err != nil {
		return err
	}
	if keyCount == 0 {
		if _, err := s.CreateAPIKey(ctx, "dev-default", []string{"chat", "models", "*"}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) countAPIKeys(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys`).Scan(&n)
	return n, err
}

func (s *Store) countProviders(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM providers`).Scan(&n)
	return n, err
}

func (s *Store) UpsertProvider(ctx context.Context, row ProviderRow) error {
	modelsJSON, err := json.Marshal(row.Models)
	if err != nil {
		return err
	}
	enabled := 0
	if row.Enabled {
		enabled = 1
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO providers (id, name, base_url, api_key, enabled, models_json, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			base_url=excluded.base_url,
			api_key=excluded.api_key,
			enabled=excluded.enabled,
			models_json=excluded.models_json,
			priority=excluded.priority
	`, row.ID, row.Name, row.BaseURL, row.APIKey, enabled, string(modelsJSON), row.Priority)
	return err
}

func (s *Store) ListProviders(ctx context.Context) ([]ProviderRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, base_url, api_key, enabled, models_json, priority FROM providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProviderRow
	for rows.Next() {
		var r ProviderRow
		var enabled int
		var modelsJSON string
		if err := rows.Scan(&r.ID, &r.Name, &r.BaseURL, &r.APIKey, &enabled, &modelsJSON, &r.Priority); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(modelsJSON), &r.Models); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetProvider(ctx context.Context, id string) (*ProviderRow, error) {
	var r ProviderRow
	var enabled int
	var modelsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, base_url, api_key, enabled, models_json, priority FROM providers WHERE id = ?
	`, id).Scan(&r.ID, &r.Name, &r.BaseURL, &r.APIKey, &enabled, &modelsJSON, &r.Priority)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(modelsJSON), &r.Models); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM providers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("provider %q not found", id)
	}
	return nil
}

func (s *Store) UpsertAlias(ctx context.Context, row AliasRow) error {
	enabled := 0
	if row.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alias_rules (id, name, match_type, pattern, rewrite, provider_id, priority, prefix, suffix, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			match_type=excluded.match_type,
			pattern=excluded.pattern,
			rewrite=excluded.rewrite,
			provider_id=excluded.provider_id,
			priority=excluded.priority,
			prefix=excluded.prefix,
			suffix=excluded.suffix,
			enabled=excluded.enabled
	`, row.ID, row.Name, string(row.MatchType), row.Pattern, row.Rewrite, row.ProviderID, row.Priority, row.Prefix, row.Suffix, enabled)
	return err
}

func (s *Store) ListAliases(ctx context.Context) ([]AliasRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, match_type, pattern, rewrite, provider_id, priority, prefix, suffix, enabled
		FROM alias_rules ORDER BY priority DESC, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AliasRow
	for rows.Next() {
		var r AliasRow
		var mt string
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &mt, &r.Pattern, &r.Rewrite, &r.ProviderID, &r.Priority, &r.Prefix, &r.Suffix, &enabled); err != nil {
			return nil, err
		}
		r.MatchType = config.MatchType(mt)
		r.Enabled = enabled == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAlias(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alias_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alias %q not found", id)
	}
	return nil
}

func (s *Store) CreateAPIKey(ctx context.Context, name string, scopes []string) (*APIKeyRow, error) {
	raw, err := generateKey()
	if err != nil {
		return nil, err
	}
	if len(scopes) == 0 {
		scopes = []string{"chat", "models"}
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, err
	}
	id, err := generateID("key")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, key_hash, scopes_json, enabled, created_at)
		VALUES (?, ?, ?, ?, 1, ?)
	`, id, name, hashKey(raw), string(scopesJSON), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &APIKeyRow{
		ID: id, Name: name, Key: raw, Scopes: scopes, Enabled: true, CreatedAt: now,
	}, nil
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKeyRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, scopes_json, enabled, created_at FROM api_keys ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKeyRow
	for rows.Next() {
		var r APIKeyRow
		var scopesJSON string
		var enabled int
		var created string
		if err := rows.Scan(&r.ID, &r.Name, &scopesJSON, &enabled, &created); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		if err := json.Unmarshal([]byte(scopesJSON), &r.Scopes); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, created)
		r.CreatedAt = t
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api key %q not found", id)
	}
	return nil
}

func (s *Store) ValidateAPIKey(ctx context.Context, raw string) (*APIKeyRow, error) {
	var r APIKeyRow
	var scopesJSON string
	var enabled int
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, scopes_json, enabled, created_at FROM api_keys WHERE key_hash = ?
	`, hashKey(raw)).Scan(&r.ID, &r.Name, &scopesJSON, &enabled, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if enabled != 1 {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(scopesJSON), &r.Scopes); err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339, created)
	r.CreatedAt = t
	r.Enabled = true
	return &r, nil
}

func (s *Store) BuildRegistry(ctx context.Context, cache ...*resolver.ModelCache) (*resolver.Registry, error) {
	providers, err := s.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	var rp []resolver.Provider
	for _, p := range providers {
		rules := make([]resolver.ModelRule, 0, len(p.Models))
		for _, m := range p.Models {
			rules = append(rules, resolver.ModelRule{MatchType: m.MatchType, Pattern: m.Pattern})
		}
		rp = append(rp, resolver.Provider{
			ID: p.ID, Name: p.Name, BaseURL: p.BaseURL, APIKey: p.APIKey,
			Enabled: p.Enabled, Rules: rules, Priority: p.Priority,
		})
	}
	var mc *resolver.ModelCache
	if len(cache) > 0 {
		mc = cache[0]
	}
	return resolver.NewRegistry(rp, mc)
}

func (s *Store) BuildAliasEngine(ctx context.Context) (*alias.Engine, error) {
	rows, err := s.ListAliases(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]alias.Rule, 0, len(rows))
	for _, r := range rows {
		rules = append(rules, alias.Rule{
			ID: r.ID, Name: r.Name, MatchType: r.MatchType, Pattern: r.Pattern,
			Rewrite: r.Rewrite, ProviderID: r.ProviderID, Priority: r.Priority,
			Prefix: r.Prefix, Suffix: r.Suffix, Enabled: r.Enabled,
		})
	}
	return alias.NewEngine(rules)
}

// --- User CRUD ---

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, email, role string) (*UserRow, error) {
	id, err := generateID("usr")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if role == "" {
		role = "user"
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, email, role, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
	`, id, username, passwordHash, email, role, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &UserRow{
		ID: id, Username: username, PasswordHash: passwordHash,
		Email: email, Role: role, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*UserRow, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, email, role, enabled, created_at, updated_at
		FROM users WHERE username = ?
	`, username))
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*UserRow, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, email, role, enabled, created_at, updated_at
		FROM users WHERE id = ?
	`, id))
}

func (s *Store) ListUsers(ctx context.Context) ([]UserRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, password_hash, email, role, enabled, created_at, updated_at
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserRow
	for rows.Next() {
		var u UserRow
		var enabled int
		var created, updated string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.Role, &enabled, &created, &updated); err != nil {
			return nil, err
		}
		u.Enabled = enabled == 1
		u.CreatedAt, _ = time.Parse(time.RFC3339, created)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id string, email *string, role *string, enabled *bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if email != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET email=?, updated_at=? WHERE id=?`, *email, now, id); err != nil {
			return err
		}
	}
	if role != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET role=?, updated_at=? WHERE id=?`, *role, now, id); err != nil {
			return err
		}
	}
	if enabled != nil {
		e := 0
		if *enabled {
			e = 1
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET enabled=?, updated_at=? WHERE id=?`, e, now, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %q not found", id)
	}
	return nil
}

func (s *Store) CreateAPIKeyForUser(ctx context.Context, userID, name string, scopes []string) (*APIKeyRow, error) {
	raw, err := generateKey()
	if err != nil {
		return nil, err
	}
	if len(scopes) == 0 {
		scopes = []string{"chat", "models"}
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, err
	}
	id, err := generateID("key")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, key_hash, scopes_json, enabled, created_at, user_id)
		VALUES (?, ?, ?, ?, 1, ?, ?)
	`, id, name, hashKey(raw), string(scopesJSON), now.Format(time.RFC3339), userID)
	if err != nil {
		return nil, err
	}
	return &APIKeyRow{
		ID: id, Name: name, Key: raw, Scopes: scopes, Enabled: true,
		UserID: userID, CreatedAt: now,
	}, nil
}

func (s *Store) scanUser(row *sql.Row) (*UserRow, error) {
	var u UserRow
	var enabled int
	var created, updated string
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.Role, &enabled, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Enabled = enabled == 1
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &u, nil
}

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func generateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "kai_" + hex.EncodeToString(b), nil
}

func generateID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b), nil
}
