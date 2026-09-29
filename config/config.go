// Package config keeps user settings in ~/.config/history-cli/config.json:
// where exports go, how the user is labelled, and which folder name each project
// was saved under last time (so the next export suggests it by default).
package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	HistoryRoot string            `json:"history_root"` // exports root, default ~/Code/history
	UserLabel   string            `json:"user_label"`   // heading for user messages, default: system login name
	Folders     map[string]string `json:"folders"`      // project path -> folder name under HistoryRoot
	LastQuery   string            `json:"last_query,omitempty"`

	path string
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// File is the config location.
func File() string { return filepath.Join(home(), ".config", "history-cli", "config.json") }

// Load reads the config, filling defaults for anything missing. It never fails:
// a broken or absent file just yields defaults.
func Load() *Config {
	c := &Config{path: File()}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, c)
	}
	if c.HistoryRoot == "" {
		c.HistoryRoot = filepath.Join(home(), "Code", "history")
	}
	if c.UserLabel == "" {
		c.UserLabel = "User"
		if u, err := user.Current(); err == nil && u.Username != "" {
			c.UserLabel = u.Username
		}
	}
	if c.Folders == nil {
		c.Folders = map[string]string{}
	}
	return c
}

// Save writes the config back to disk.
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, append(data, '\n'), 0o644)
}

var unsafeChars = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// FolderFor suggests the export folder for a project: the remembered one, else the
// project's directory name (e.g. ~/Code/my-app -> "my-app").
func (c *Config) FolderFor(projectPath string) string {
	for p, f := range c.Folders {
		if strings.EqualFold(p, projectPath) {
			return f
		}
	}
	name := unsafeChars.ReplaceAllString(filepath.Base(projectPath), "-")
	if name == "" || name == "." || name == "-" {
		return "misc"
	}
	return strings.Trim(name, "-")
}

// Remember stores the folder chosen for a project and saves the config.
func (c *Config) Remember(projectPath, folder string) {
	for p := range c.Folders {
		if strings.EqualFold(p, projectPath) {
			delete(c.Folders, p)
		}
	}
	c.Folders[projectPath] = folder
	_ = c.Save()
}
