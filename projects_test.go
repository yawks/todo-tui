package main

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestProjectHierarchy(t *testing.T) {
	projects := []project{{UID: "child", ParentUID: "parent"}, {UID: "orphan", ParentUID: "missing"}, {UID: "parent"}, {UID: "a", ParentUID: "b"}, {UID: "b", ParentUID: "a"}}
	ordered := orderedProjects(projects)
	if len(ordered) != len(projects) {
		t.Fatal("projects lost")
	}
	seen := map[string]bool{}
	for i, p := range ordered {
		if seen[p.UID] {
			t.Fatal("duplicate project")
		}
		seen[p.UID] = true
		if p.UID == "child" && (i == 0 || ordered[i-1].UID != "parent" || p.Depth != 1) {
			t.Fatal("child not grouped under parent")
		}
		if p.UID == "orphan" && p.Depth != 0 {
			t.Fatal("orphan hidden")
		}
	}
	if !projectDescendsFrom(projects, "child", "parent") || !projectDescendsFrom(projects, "a", "new") || projectDescendsFrom(projects, "parent", "child") {
		t.Fatal("invalid ancestry check")
	}
	for _, kind := range []string{"tududi", "open-todo"} {
		m := model{api: newBackend(config{Backend: kind})}
		m.editor = &editor{kind: editProject}
		m.addProjectParentField()
		if (len(m.editor.fields) == 1) != (kind == "open-todo") {
			t.Fatal("incorrect backend capability")
		}
	}
}

func TestSelectedProjectIncludesDescendants(t *testing.T) {
	m := model{api: newBackend(config{Backend: "open-todo"}), selectedProjectUID: "parent", projects: []project{
		{ID: 1, UID: "parent"}, {ID: 2, UID: "child", ParentUID: "parent"}, {ID: 3, UID: "grandchild", ParentUID: "child"},
		{UID: "other"}, {UID: "a", ParentUID: "b"}, {UID: "b", ParentUID: "a"},
	}}
	for _, tc := range []struct {
		task task
		want bool
	}{
		{task{ProjectUID: "parent"}, true},
		{task{ProjectUID: "child"}, true},
		{task{ProjectID: 3}, true},
		{task{Project: &project{UID: "grandchild", ParentUID: "child"}}, true},
		{task{ProjectUID: "other"}, false},
		{task{ProjectUID: "missing"}, false},
		{task{ProjectUID: "a"}, false},
		{task{}, false},
	} {
		if got := m.matchesSelectedProject(tc.task); got != tc.want {
			t.Errorf("task %+v: got %v, want %v", tc.task, got, tc.want)
		}
	}
	m.selectedProjectUID = "child"
	if m.matchesSelectedProject(task{ProjectUID: "parent"}) {
		t.Fatal("child includes ancestor")
	}
	m.api = newBackend(config{Backend: "tududi"})
	if m.matchesSelectedProject(task{ProjectUID: "grandchild"}) {
		t.Fatal("unsupported backend includes descendants")
	}
}

func TestGroupedTaskListShowsProject(t *testing.T) {
	m := model{api: newBackend(config{Backend: "open-todo"}), selectedProjectUID: "parent", projects: []project{
		{UID: "parent", Name: "Group"}, {UID: "child", ParentUID: "parent", Name: "Child"}, {UID: "grandchild", ParentUID: "child", Name: "Grandchild"},
	}}
	for _, p := range m.projects {
		for _, active := range []bool{true, false} {
			line := ansi.Strip(m.taskListItem(task{Name: "Task", ProjectUID: p.UID}, 60, active))
			if !strings.Contains(line, "● "+p.Name) {
				t.Fatalf("missing project in group: %q", line)
			}
		}
	}
	m.selectedProjectUID = "grandchild"
	if strings.Contains(ansi.Strip(m.taskListItem(task{Name: "Task", ProjectUID: "grandchild"}, 60, false)), "● Grandchild") {
		t.Fatal("leaf project label should remain hidden")
	}
}
