package main

// backend translates server data into the models used by the terminal UI.
type backend interface {
	load() ([]task, []project, []tag, error)
	saveTask(task, bool) error
	toggleTask(task) error
	setTaskStatus(task, string) error
	deleteTask(task) error
	saveProject(project, bool) error
	deleteProject(project) error
	saveTag(tag, bool) error
	deleteTag(tag) error
	configuration() config
}

func (c *client) configuration() config { return c.cfg }

func newBackend(cfg config) backend {
	c := newClient(cfg)
	if cfg.Backend == "open-todo" {
		c.prefix = "/api/v1/"
		return &openTodoClient{client: c}
	}
	return c
}

func (c *client) supportsProjectHierarchy() bool         { return false }
func (c *openTodoClient) supportsProjectHierarchy() bool { return true }

func projectHierarchySupported(api backend) bool {
	c, ok := api.(interface{ supportsProjectHierarchy() bool })
	return ok && c.supportsProjectHierarchy()
}
