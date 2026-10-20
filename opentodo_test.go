package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenTodoLoadAndSave(t *testing.T) {
	var saved map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ot_test" {
			t.Error("missing token")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/tasks":
			_, _ = w.Write([]byte(`{"tasks":[{"id":"task-id","title":"Example","description":"note","project_id":"project-id","priority":3,"due":{"date":"2026-10-20","time":"14:30","tz":"Europe/Paris"},"labels":["label-id"],"created_at":1792454400000,"completed_at":null}]}`))
		case "GET /api/v1/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"project-id","name":"Work","color":"#ff0000"},{"id":"other","name":"Other"}]}`))
		case "GET /api/v1/labels":
			_, _ = w.Write([]byte(`{"labels":[{"id":"label-id","name":"Urgent","color":"#ff0000"}]}`))
		case "PATCH /api/v1/tasks/task-id":
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	c := newBackend(config{Backend: "open-todo", BaseURL: server.URL, APIKey: "ot_test"})
	tasks, projects, tags, err := c.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || len(projects) != 2 || len(tags) != 1 {
		t.Fatal("missing data")
	}
	task := tasks[0]
	if task.UID != "task-id" || task.ProjectUID != "project-id" || task.Tags[0].Name != "Urgent" || statusName(task.Status) != "planned" {
		t.Fatalf("task = %+v", task)
	}
	m := newModel(c, appState{ProjectUID: "project-id"})
	m.projects = projects
	if !m.matchesSelectedProject(task) {
		t.Fatal("project missing")
	}
	m.selectedProjectUID = "other"
	if m.matchesSelectedProject(task) {
		t.Fatal("incorrect project association")
	}
	if err := c.saveTask(task, false); err != nil {
		t.Fatal(err)
	}
	if saved["title"] != "Example" || saved["project_id"] != "project-id" || saved["priority"] != float64(3) || saved["completed_at"] != nil {
		t.Fatalf("payload = %#v", saved)
	}
	due := saved["due"].(map[string]any)
	if due["time"] != "14:30" || due["tz"] != "Europe/Paris" {
		t.Fatal("due time lost")
	}
	if saved["labels"].([]any)[0] != "label-id" {
		t.Fatal("label id lost")
	}
	if err := c.toggleTask(task); err != nil {
		t.Fatal(err)
	}
	if saved["completed_at"] == nil {
		t.Fatal("completion missing")
	}
	if err := c.setTaskStatus(task, "waiting"); err == nil {
		t.Fatal("unsupported status accepted")
	}
}

func TestOpenTodoWrites(t *testing.T) {
	var method, path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body = nil
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	c := newBackend(config{Backend: "open-todo", BaseURL: server.URL})
	cases := []struct {
		name, method, path string
		run                func() error
	}{
		{"create project", "POST", "/api/v1/projects", func() error { return c.saveProject(project{Name: "Work"}, true) }},
		{"archive project", "PATCH", "/api/v1/projects/p", func() error { return c.saveProject(project{UID: "p", Name: "Work", Status: "done"}, false) }},
		{"create label", "POST", "/api/v1/labels", func() error { return c.saveTag(tag{Name: "Urgent", Color: "#ff0000"}, true) }},
		{"edit label", "PATCH", "/api/v1/labels/l", func() error { return c.saveTag(tag{UID: "l", Name: "New"}, false) }},
		{"delete task", "DELETE", "/api/v1/tasks/t", func() error { return c.deleteTask(task{UID: "t"}) }},
		{"delete project", "DELETE", "/api/v1/projects/p", func() error { return c.deleteProject(project{UID: "p"}) }},
		{"delete label", "DELETE", "/api/v1/labels/l", func() error { return c.deleteTag(tag{UID: "l"}) }},
		{"reopen task", "PATCH", "/api/v1/tasks/t", func() error { return c.toggleTask(task{UID: "t", Status: "done"}) }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatal(err)
			}
			if method != tt.method || path != tt.path {
				t.Fatalf("got %s %s", method, path)
			}
			if tt.name == "archive project" && body["archived"] != true {
				t.Fatal("archive missing")
			}
			if tt.name == "reopen task" {
				if v, ok := body["completed_at"]; !ok || v != nil {
					t.Fatal("completion not cleared")
				}
			}
		})
	}
}
