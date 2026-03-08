// Package credential manages Slack credentials for slagent.
package credential

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Credentials holds the stored Slack token for one workspace.
type Credentials struct {
	Token  string `json:"token,omitempty"`
	Type   string `json:"type,omitempty"`   // "bot", "user", or "session"
	Cookie string `json:"cookie,omitempty"` // xoxd-... for xoxc session tokens
}

// EffectiveToken returns the token.
func (c *Credentials) EffectiveToken() string {
	return c.Token
}

// EffectiveType returns the token type, inferring from prefix if not set.
func (c *Credentials) EffectiveType() string {
	if c.Type != "" {
		return c.Type
	}
	switch {
	case strings.HasPrefix(c.Token, "xoxp-"):
		return "user"
	case strings.HasPrefix(c.Token, "xoxc-"):
		return "session"
	default:
		return "bot"
	}
}

// store is the on-disk format for credentials.json.
type store struct {
	Default    string                 `json:"default"`
	Workspaces map[string]Credentials `json:"workspaces"`
}

// Dir returns the credentials directory.
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "slagent")
}

// Path returns the path to the credentials file.
func Path() string {
	return filepath.Join(Dir(), "credentials.json")
}

func loadStore() (*store, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		return nil, err
	}
	var s store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return &s, nil
}

func saveStore(s *store) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads stored credentials. Empty workspace uses the default.
func Load(workspace string) (*Credentials, error) {
	s, err := loadStore()
	if err != nil {
		return nil, fmt.Errorf("no credentials found (run 'slaude auth'): %w", err)
	}

	name := workspace
	if name == "" {
		name = s.Default
	}
	if name == "" {
		return nil, fmt.Errorf("no default workspace set (run 'slaude auth')")
	}

	creds, ok := s.Workspaces[name]
	if !ok {
		return nil, fmt.Errorf("workspace %q not found (run 'slaude auth')", name)
	}
	if creds.EffectiveToken() == "" {
		return nil, fmt.Errorf("empty token for workspace %q (run 'slaude auth')", name)
	}
	return &creds, nil
}

// Save writes credentials for a workspace. Sets as default if it's the first
// workspace or if setDefault is true.
func Save(name string, creds *Credentials) error {
	s, err := loadStore()
	if err != nil {
		s = &store{Workspaces: make(map[string]Credentials)}
	}
	if s.Workspaces == nil {
		s.Workspaces = make(map[string]Credentials)
	}

	s.Workspaces[name] = *creds

	// Set default if first workspace or no default set
	if s.Default == "" {
		s.Default = name
	}

	return saveStore(s)
}

// SetDefault sets the default workspace.
func SetDefault(name string) error {
	s, err := loadStore()
	if err != nil {
		return fmt.Errorf("no credentials found (run 'slaude auth'): %w", err)
	}
	if _, ok := s.Workspaces[name]; !ok {
		return fmt.Errorf("workspace %q not found", name)
	}
	s.Default = name
	return saveStore(s)
}

// ListWorkspaces returns all workspace names sorted, with the default first.
func ListWorkspaces() (names []string, defaultName string, err error) {
	s, err := loadStore()
	if err != nil {
		return nil, "", nil
	}
	for name := range s.Workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, s.Default, nil
}
