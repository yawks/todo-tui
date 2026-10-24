package main

// Missing parents and cycles stay visible at the top level.
func orderedProjects(projects []project) []project {
	result := make([]project, 0, len(projects))
	visited := make([]bool, len(projects))
	var visit func(int, int)
	visit = func(i, depth int) {
		if visited[i] {
			return
		}
		visited[i] = true
		p := projects[i]
		p.Depth = depth
		result = append(result, p)
		for j, child := range projects {
			if p.UID != "" && child.ParentUID == p.UID {
				visit(j, depth+1)
			}
		}
	}
	for i, p := range projects {
		found := false
		for _, parent := range projects {
			if p.ParentUID != "" && parent.UID == p.ParentUID && parent.Status != "done" {
				found = true
				break
			}
		}
		if !found {
			visit(i, 0)
		}
	}
	for i := range projects {
		visit(i, 0)
	}
	return result
}

func projectDescendsFrom(projects []project, uid, ancestor string) bool {
	seen := map[string]bool{}
	for uid != "" {
		if uid == ancestor || seen[uid] {
			return true
		}
		seen[uid] = true
		parent := ""
		for _, p := range projects {
			if p.UID == uid {
				parent = p.ParentUID
				break
			}
		}
		uid = parent
	}
	return false
}

func (m *model) addProjectParentField() {
	if !projectHierarchySupported(m.api) {
		return
	}
	value := m.editor.project.ParentUID
	// Keep the UID when names are ambiguous.
	for _, p := range m.projects {
		if p.UID == value {
			count := 0
			for _, other := range m.projects {
				if other.Name == p.Name {
					count++
				}
			}
			if count == 1 {
				value = p.Name
			}
			break
		}
	}
	m.editor.fields = append(m.editor.fields, input(value, "Parent project"))
}
