package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) openConfig() {
	cfg := m.api.configuration()
	if cfg.Backend == "" {
		cfg.Backend = "tududi"
	}
	key := input(cfg.APIKey, "API key")
	key.CharLimit = 0
	key.EchoMode = textinput.EchoPassword
	m.err = ""
	m.editor = &editor{kind: editConfig, fields: []textinput.Model{input(cfg.Backend, "tududi / open-todo"), input(cfg.BaseURL, "https://todo.example.com"), key}}
}

func (m model) saveConfigEditor() (tea.Model, tea.Cmd) {
	e := m.editor
	cfg, err := normalizeConfig(config{Backend: e.fields[0].Value(), BaseURL: e.fields[1].Value(), APIKey: e.fields[2].Value()})
	if err == nil {
		err = saveConfig(cfg)
	}
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	state := m.persistedState()
	old := m.api.configuration()
	if old.Backend != cfg.Backend || old.BaseURL != cfg.BaseURL {
		state = appState{Filter: "today"}
	}
	next := newModel(newBackend(cfg), state)
	next.width, next.height = m.width, m.height
	next.status = "Configuration saved"
	return next, tea.Batch(next.loadCmd(), next.saveStateCmd())
}
