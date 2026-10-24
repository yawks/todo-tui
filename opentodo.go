package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type openTodoClient struct{ *client }

type openTodoDue struct {
	Date string `json:"date"`
	Time string `json:"time,omitempty"`
	TZ   string `json:"tz,omitempty"`
}

type openTodoTask struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	ProjectID   string       `json:"project_id"`
	Priority    int          `json:"priority"`
	Due         *openTodoDue `json:"due"`
	Labels      []string     `json:"labels"`
	CreatedAt   int64        `json:"created_at"`
	CompletedAt *int64       `json:"completed_at"`
}

type openTodoProject struct {
	ParentID    string `json:"parent_id"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Archived    bool   `json:"archived"`
}
type openTodoLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (c *openTodoClient) load() ([]task, []project, []tag, error) {
	var ts struct {
		Tasks []openTodoTask `json:"tasks"`
	}
	var ps struct {
		Projects []openTodoProject `json:"projects"`
	}
	var ls struct {
		Labels []openTodoLabel `json:"labels"`
	}
	if err := c.do(http.MethodGet, "tasks", nil, &ts); err != nil {
		return nil, nil, nil, err
	}
	if err := c.do(http.MethodGet, "projects", nil, &ps); err != nil {
		return nil, nil, nil, err
	}
	if err := c.do(http.MethodGet, "labels", nil, &ls); err != nil {
		return nil, nil, nil, err
	}
	tasks := make([]task, 0, len(ts.Tasks))
	projects := make([]project, 0, len(ps.Projects))
	tags := make([]tag, 0, len(ls.Labels))
	labels := map[string]tag{}
	for _, l := range ls.Labels {
		t := tag{UID: l.ID, Name: l.Name, Color: l.Color}
		tags = append(tags, t)
		labels[l.ID] = t
	}
	for _, p := range ps.Projects {
		status := "not_started"
		if p.Archived {
			status = "done"
		}
		projects = append(projects, project{ParentUID: p.ParentID, UID: p.ID, Name: p.Name, Description: p.Description, Color: p.Color, Status: status})
	}
	for _, v := range ts.Tasks {
		priority := "low"
		if v.Priority == 1 {
			priority = "high"
		} else if v.Priority == 2 {
			priority = "medium"
		}
		t := task{UID: v.ID, Name: v.Title, Note: v.Description, ProjectUID: v.ProjectID, Priority: priority, Status: "not_started", CreatedAt: time.UnixMilli(v.CreatedAt).UTC().Format(time.RFC3339), OpenTodoPriority: v.Priority, OpenTodoDue: v.Due}
		if v.Due != nil {
			t.DueDate = v.Due.Date
			t.Status = "planned"
		}
		if v.CompletedAt != nil {
			t.Status = "done"
			t.CompletedAt = time.UnixMilli(*v.CompletedAt).UTC().Format(time.RFC3339)
		}
		for _, id := range v.Labels {
			if l, ok := labels[id]; ok {
				t.Tags = append(t.Tags, l)
			}
		}
		tasks = append(tasks, t)
	}
	return tasks, orderedProjects(projects), tags, nil
}

func openTodoCompletion(t task, status string) (any, error) {
	switch status {
	case "done":
		if t.CompletedAt != "" {
			if d, err := time.Parse(time.RFC3339, t.CompletedAt); err == nil {
				return d.UnixMilli(), nil
			}
		}
		return time.Now().UnixMilli(), nil
	case "not_started", "planned":
		return nil, nil
	default:
		return nil, fmt.Errorf("OpenTodo supports open or completed tasks; status %q is unavailable", status)
	}
}

func (c *openTodoClient) saveTask(t task, create bool) error {
	completed, err := openTodoCompletion(t, statusName(t.Status))
	if err != nil {
		return err
	}
	priority := map[string]int{"high": 1, "medium": 2, "low": 4}[priorityName(t.Priority)]
	// Preserve OpenTodo's fourth priority level when the displayed priority is unchanged.
	if t.OpenTodoPriority == 3 && priorityName(t.Priority) == "low" {
		priority = 3
	}
	var due any
	if t.DueDate != "" {
		d, ok := t.due()
		if !ok {
			return fmt.Errorf("invalid due date")
		}
		value := openTodoDue{Date: d.Format("2006-01-02")}
		if t.OpenTodoDue != nil {
			value.Time = t.OpenTodoDue.Time
			value.TZ = t.OpenTodoDue.TZ
		}
		due = value
	}
	var ls struct {
		Labels []openTodoLabel `json:"labels"`
	}
	if err := c.do(http.MethodGet, "labels", nil, &ls); err != nil {
		return err
	}
	ids := make([]string, 0, len(t.Tags))
	for _, tag := range t.Tags {
		found := false
		for _, l := range ls.Labels {
			if strings.EqualFold(l.Name, tag.Name) {
				ids = append(ids, l.ID)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown OpenTodo label %q; create it first with T", tag.Name)
		}
	}
	var projectID any
	if t.ProjectUID != "" {
		projectID = t.ProjectUID
	}
	payload := map[string]any{"title": t.Name, "description": t.Note, "project_id": projectID, "priority": priority, "due": due, "labels": ids, "completed_at": completed}
	return c.save("tasks", t.UID, create, payload)
}
func (c *openTodoClient) save(collection, id string, create bool, payload any) error {
	method, path := http.MethodPost, collection
	if !create {
		method, path = http.MethodPatch, collection+"/"+ident(id, 0)
	}
	return c.do(method, path, payload, nil)
}
func (c *openTodoClient) toggleTask(t task) error {
	status := "done"
	if t.completed() {
		status = "not_started"
	}
	return c.setTaskStatus(t, status)
}
func (c *openTodoClient) setTaskStatus(t task, status string) error {
	completed, err := openTodoCompletion(t, status)
	if err != nil {
		return err
	}
	return c.save("tasks", t.UID, false, map[string]any{"completed_at": completed})
}
func (c *openTodoClient) deleteTask(t task) error {
	return c.do(http.MethodDelete, "tasks/"+ident(t.UID, 0), nil, nil)
}
func (c *openTodoClient) saveProject(p project, create bool) error {
	if p.Status != "" && p.Status != "not_started" && p.Status != "done" {
		return fmt.Errorf("OpenTodo project status must be not_started (active) or done (archived)")
	}
	var parent any
	if p.ParentUID != "" {
		parent = p.ParentUID
	}
	return c.save("projects", p.UID, create, map[string]any{"parent_id": parent, "name": p.Name, "description": p.Description, "color": p.Color, "archived": p.Status == "done"})
}
func (c *openTodoClient) deleteProject(p project) error {
	return c.do(http.MethodDelete, "projects/"+ident(p.UID, 0), nil, nil)
}
func (c *openTodoClient) saveTag(t tag, create bool) error {
	return c.save("labels", t.UID, create, map[string]any{"name": t.Name, "color": t.Color})
}
func (c *openTodoClient) deleteTag(t tag) error {
	return c.do(http.MethodDelete, "labels/"+ident(t.UID, 0), nil, nil)
}
