package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"strings"
	"testing"
)

func TestNormalizeConfig(t *testing.T) {
	cfg, err := normalizeConfig(config{BaseURL: " https://example.com/ ", APIKey: " key "})
	if err != nil || cfg.Backend != "tududi" || cfg.BaseURL != "https://example.com" || cfg.APIKey != "key" {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	for _, cfg := range []config{
		{Backend: "unknown", BaseURL: "https://example.com", APIKey: "key"},
		{BaseURL: "file:///tmp/tasks", APIKey: "key"},
		{BaseURL: "https://example.com?token=secret", APIKey: "key"},
		{BaseURL: "https://example.com"},
	} {
		if _, err := normalizeConfig(cfg); err == nil {
			t.Fatalf("invalid config accepted: %+v", cfg)
		}
	}
}

func TestConfigEditorSavesAndSwitchesBackend(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newModel(newBackend(config{BaseURL: "https://old.example", APIKey: "secret"}), appState{})
	m.width, m.height, m.loading = 100, 30, false
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = next.(model)
	if m.editor == nil || !strings.Contains(m.View(), "configuration") || strings.Contains(m.View(), "secret") {
		t.Fatal("configuration not displayed or secret visible")
	}
	m.editor.fields[0].SetValue("open-todo")
	m.editor.fields[1].SetValue("https://new.example/")
	next, cmd := m.saveEditor()
	m = next.(model)
	if cmd == nil || m.editor != nil {
		t.Fatal("configuration was not applied")
	}
	if _, ok := m.api.(*openTodoClient); !ok {
		t.Fatal("backend not switched")
	}
	cfg, err := loadConfig()
	if err != nil || cfg.Backend != "open-todo" || cfg.BaseURL != "https://new.example" {
		t.Fatalf("saved config = %+v, %v", cfg, err)
	}
	path, _ := configPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v", info.Mode())
	}
}
