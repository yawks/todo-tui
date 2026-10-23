package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type page int

const (
	pageToday page = iota
	pageUpcoming
	pageUnplanned
	pageDone
	pageCalendar
	pageTags
	pageAll
	pageDashboard
	pageStatistics
)

const (
	sideDashboard = iota
	sideAllFilters
	sideToday
	sideUpcoming
	sideUnplanned
	sideDone
	sideCalendar
	sideTags
	sideStatistics
	sideAllProjects
	sideFirstProject
)

var labels = struct {
	Dashboard, Today, Upcoming, Unplanned, Done, Calendar, Tags, Projects, Help, Empty string
}{"Dashboard", "Today", "Upcoming", "Unplanned", "Done", "Calendar", "Tags", "Projects", "? help", "Nothing here"}

type loadedMsg struct {
	tasks    []task
	projects []project
	tags     []tag
	err      error
}
type actionMsg struct{ err error }
type taskToggleMsg struct {
	task     task
	previous *task
	undo     bool
	err      error
}
type celebrationTickMsg struct{ generation int }

func celebrationTick(generation int) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return celebrationTickMsg{generation} })
}

type stateSavedMsg struct{ err error }
type editorKind int

const (
	editTask editorKind = iota
	editProject
	editTag
	editConfig
)

type taskSearch struct {
	allTasks bool
	focusAll bool
	input    textinput.Model
	cursor   int
}

type editor struct {
	readOnly    bool
	kind        editorKind
	create      bool
	fields      []textinput.Model
	note        textarea.Model
	focus       int
	task        task
	project     project
	tag         tag
	calendar    bool
	colorPicker bool
	colorIndex  int
	choice      int
	choiceIndex int
	date        time.Time
}

var colorPalette = []string{
	"#fecaca", "#fca5a5", "#f87171", "#ef4444", "#dc2626", "#991b1b",
	"#fed7aa", "#fdba74", "#fb923c", "#f97316", "#ea580c", "#9a3412",
	"#fef08a", "#fde047", "#facc15", "#eab308", "#ca8a04", "#854d0e",
	"#bbf7d0", "#86efac", "#4ade80", "#22c55e", "#16a34a", "#166534",
	"#99f6e4", "#5eead4", "#2dd4bf", "#14b8a6", "#0d9488", "#115e59",
	"#bae6fd", "#7dd3fc", "#38bdf8", "#0ea5e9", "#0284c7", "#075985",
	"#c7d2fe", "#a5b4fc", "#818cf8", "#6366f1", "#4f46e5", "#3730a3",
	"#e9d5ff", "#d8b4fe", "#c084fc", "#a855f7", "#9333ea", "#6b21a8",
	"#fbcfe8", "#f9a8d4", "#f472b6", "#ec4899", "#db2777", "#9d174d",
	"#e5e7eb", "#9ca3af", "#6b7280", "#374151", "#111827", "#000000",
}

type model struct {
	api                backend
	tasks              []task
	projects           []project
	tags               []tag
	page               page
	selectedProjectUID string
	lastFilter         string
	side, cursor       int
	focusSide          bool
	showCompleted      bool
	week               bool
	workingWeek        bool
	calendarTasks      bool
	calDate            time.Time
	width, height      int
	togglePending      bool
	loading            bool
	loader             spinner.Model
	err                string
	status             string
	search             *taskSearch
	editor             *editor
	confirm            bool
	help               bool
	menu               string
	menuIndex          int
	sortBy             string
	sortDesc           bool
	priorityFilter     string
	statusFilter       string
	tagFilters         map[string]bool
	doneFrom, doneTo   time.Time
	doneDate           time.Time
	donePicking        int
	detailScroll       int
	undoTask           *task
	toast              string
	toastFrame         int
	statsPeriod        int
	statsCursor        int
	statsPeriodCursor  int
	statsTasksFocus    bool
	toastGeneration    int
}

var (
	blue       = lipgloss.Color("#3b82f6")
	muted      = lipgloss.Color("#8b949e")
	panel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#30363d")).Padding(0, 1)
	selected   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(lipgloss.Color("#1f6feb")).Bold(true)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#1f2937", Dark: "#f0f6fc"})
	dim        = lipgloss.NewStyle().Foreground(muted)
)

func newModel(api backend, state appState) model {
	now := time.Now()
	p := pageFromFilter(state.Filter)
	side := sideAllFilters
	if p <= pageDone {
		side = int(p) + sideToday
	} else if p == pageStatistics {
		side = sideStatistics
	} else if p == pageDashboard {
		side = sideDashboard
	}
	tagFilters := selectedNames(state.Tags)
	today := dayStart(now)
	return model{api: api, loader: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(blue))), week: state.CalendarWeek, workingWeek: state.CalendarWorkingWeek, page: p, side: side, selectedProjectUID: state.ProjectUID, lastFilter: state.Filter, focusSide: true, loading: true, showCompleted: p == pageDone, calDate: today, doneFrom: today, doneTo: today.AddDate(0, 0, 1), sortBy: state.Sort, sortDesc: state.SortDesc, priorityFilter: state.Priority, statusFilter: state.Status, tagFilters: tagFilters}
}

func pageFromFilter(filter string) page {
	switch filter {
	case "upcoming":
		return pageUpcoming
	case "unplanned":
		return pageUnplanned
	case "done":
		return pageDone
	case "all":
		return pageAll
	case "statistics":
		return pageStatistics
	case "dashboard":
		return pageDashboard
	default:
		return pageToday
	}
}

func (m model) persistedState() appState {
	filter := m.lastFilter
	if filter == "" && m.page <= pageDone {
		filter = []string{"today", "upcoming", "unplanned", "done"}[m.page]
	}
	if filter == "" {
		filter = "today"
	}
	var tags []string
	for _, tag := range m.tags {
		if m.tagFilters[strings.ToLower(tag.Name)] {
			tags = append(tags, tag.Name)
		}
	}
	return appState{CalendarWeek: m.week, CalendarWorkingWeek: m.workingWeek, ProjectUID: m.selectedProjectUID, Filter: filter, Sort: m.sortBy, SortDesc: m.sortDesc, Priority: m.priorityFilter, Status: m.statusFilter, Tags: strings.Join(tags, ",")}
}

func (m model) saveStateCmd() tea.Cmd {
	state := m.persistedState()
	return func() tea.Msg { return stateSavedMsg{saveState(state)} }
}

func (m model) Init() tea.Cmd { return m.loadCmd() }
func (m model) loadCmd() tea.Cmd {
	return tea.Batch(m.loader.Tick, func() tea.Msg { t, p, g, err := m.api.load(); return loadedMsg{t, p, g, err} })
}
func action(fn func() error) tea.Cmd { return func() tea.Msg { return actionMsg{fn()} } }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.loader, cmd = m.loader.Update(msg)
		return m, cmd
	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.tasks, m.projects, m.tags, m.err = msg.tasks, msg.projects, msg.tags, ""
		}
		m.clamp()
		if m.search != nil && m.editor != nil && m.editor.readOnly {
			for _, t := range m.tasks {
				if t.ID == m.editor.task.ID && t.UID == m.editor.task.UID {
					m.openTaskEditor(t)
					m.editor.readOnly = true
					break
				}
			}
		}
		if m.selectedProjectUID != "" && m.selectedProject() == nil {
			m.selectedProjectUID = ""
			return m, m.saveStateCmd()
		}
		return m, nil
	case celebrationTickMsg:
		if msg.generation != m.toastGeneration || m.toast == "" {
			return m, nil
		}
		m.toastFrame++
		if m.toastFrame >= 60 {
			m.toast, m.undoTask = "", nil
			return m, nil
		}
		return m, celebrationTick(m.toastGeneration)
	case taskToggleMsg:
		m.togglePending = false
		if msg.err != nil {
			previous := msg.task
			if msg.previous != nil {
				previous = *msg.previous
			}
			m.replaceTask(previous)
			m.loading = false
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.toastGeneration++
		m.toastFrame = 0
		if msg.undo {
			m.undoTask = nil
			m.toast = "Completion undone"
		} else if !msg.task.completed() {
			t := msg.task
			m.undoTask = &t
			m.toast = "Well done 🎉"
		} else {
			m.undoTask = nil
			m.toast = "Task reopened"
		}
		return m, celebrationTick(m.toastGeneration)
	case actionMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		if m.search != nil && m.editor != nil {
			m.editor.readOnly = true
		} else {
			m.editor = nil
		}
		m.confirm, m.status = false, "Saved"
		m.loading = true
		return m, m.loadCmd()
	case stateSavedMsg:
		if msg.err != nil {
			m.err = "save state: " + msg.err.Error()
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.editor != nil {
		return m.updateEditor(key)
	}
	if m.search != nil {
		return m.updateSearch(key)
	}
	if m.confirm {
		return m.updateConfirm(key)
	}
	if m.help {
		if key.String() == "?" || key.String() == "esc" {
			m.help = false
		}
		return m, nil
	}
	if m.menu != "" {
		return m.updateMenu(key)
	}
	if m.page == pageStatistics && !m.focusSide {
		if m.updateStatisticsKey(key.String()) {
			return m, nil
		}
	}
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "c":
		if !m.loading {
			m.openConfig()
			return m, m.editor.fields[0].Focus()
		}
	case "?":
		m.help = true
	case "/":
		m.search = &taskSearch{input: input("", "Search tasks…")}
		return m, m.search.input.Focus()
	case "s":
		m.menu, m.menuIndex = "sort", max(0, slices.Index([]string{"date", "name", "priority", "tag"}, m.sortBy))
	case "f":
		m.menu, m.menuIndex = "filter", 0
	case "tab", "shift+tab":
		m.focusSide = !m.focusSide
	case "h":
		m.showCompleted = !m.showCompleted
		m.cursor = 0
	case "r":
		m.loading = true
		return m, m.loadCmd()
	case "v":
		if m.page == pageCalendar {
			if !m.week {
				m.week = true
			} else if !m.workingWeek {
				m.workingWeek = true
			} else {
				m.week, m.workingWeek = false, false
			}
			return m, m.saveStateCmd()
		}
	case "left":
		if m.page == pageCalendar && !m.focusSide {
			if m.calendarTasks {
				m.calendarTasks = false
			} else {
				m.moveCalendarHorizontal(-1)
			}
		} else if !m.focusSide {
			m.focusSide = true
		}
	case "right":
		if m.page == pageCalendar && !m.focusSide && !m.calendarTasks {
			m.moveCalendarHorizontal(1)
		} else if m.focusSide {
			m.focusSide = false
		}
	case "[":
		if m.page == pageCalendar && !m.focusSide {
			m.moveCalendar(-1)
		}
	case "]":
		if m.page == pageCalendar && !m.focusSide {
			m.moveCalendar(1)
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "pgdown", "home", "end":
		if !m.focusSide {
			m.scrollTaskDetail(key.String())
		}
	case "enter":
		if m.focusSide {
			cmd := m.chooseSide()
			return m, cmd
		}
		if m.page == pageTags {
			m.openEdit()
		} else if m.page == pageCalendar && len(m.tasksDueOn(m.calDate)) > 0 {
			if m.calendarTasks {
				m.openEdit()
			} else {
				m.calendarTasks = true
				m.cursor = 0
			}
		}
	case "esc":
		m.calendarTasks = false
	case "n":
		m.openNew()
	case "N":
		m.editor = &editor{kind: editProject, create: true, fields: []textinput.Model{input("", "Name"), input("#3b82f6", "#RRGGBB"), input("not_started", "Status")}, note: note("", "Description")}
		m.editor.fields[0].Focus()
	case "T":
		m.editor = &editor{kind: editTag, create: true, fields: []textinput.Model{input("", "Name"), input("#3b82f6", "#RRGGBB")}}
		m.editor.fields[0].Focus()
	case "e":
		m.openEdit()
	case "d":
		if m.selectedExists() {
			m.confirm = true
		}
	case "u":
		if m.undoTask != nil && !m.loading && !m.togglePending {
			t := *m.undoTask
			var previous *task
			for _, current := range m.tasks {
				if current.ID == t.ID && current.UID == t.UID {
					previous = &current
					break
				}
			}
			m.replaceTask(t)
			m.togglePending = true
			return m, func() tea.Msg {
				return taskToggleMsg{task: t, previous: previous, undo: true, err: m.api.setTaskStatus(t, statusName(t.Status))}
			}
		}
	case " ":
		if t, ok := m.selectedTask(); ok && !m.loading && !m.togglePending {
			updated := t
			updated.Status, updated.CompletedAt = "not_started", ""
			if !t.completed() {
				updated.Status, updated.CompletedAt = "done", time.Now().Format(time.RFC3339)
			}
			m.replaceTask(updated)
			m.togglePending = true
			return m, func() tea.Msg { return taskToggleMsg{task: t, err: m.api.toggleTask(t)} }
		}
	}
	return m, nil
}

func (m *model) replaceTask(updated task) {
	for i, t := range m.tasks {
		if t.ID == updated.ID && t.UID == updated.UID {
			m.tasks[i] = updated
			break
		}
	}
	m.clamp()
}

func (m model) searchResults() []task {
	filtered := m
	if m.page == pageCalendar || m.page == pageTags || m.page == pageDashboard {
		filtered.page = pageFromFilter(m.lastFilter)
	}
	query := strings.ToLower(strings.TrimSpace(m.search.input.Value()))
	var results []task
	tasks := filtered.filteredTasks()
	if m.search.allTasks {
		tasks = m.tasks
	}
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Name+"\n"+t.Note), query) {
			results = append(results, t)
		}
	}
	return results
}

func (m model) updateSearch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "tab" || key.String() == "shift+tab" {
		m.search.focusAll = !m.search.focusAll
		if m.search.focusAll {
			m.search.input.Blur()
		} else {
			return m, m.search.input.Focus()
		}
		return m, nil
	}
	if m.search.focusAll && (key.String() == " " || key.String() == "enter") {
		m.search.allTasks = !m.search.allTasks
		m.search.cursor = 0
		return m, nil
	}
	if m.search.focusAll && (key.String() == "up" || key.String() == "down") {
		m.search.focusAll = false
		return m, m.search.input.Focus()
	}
	results := m.searchResults()
	m.search.cursor = max(0, min(m.search.cursor, len(results)-1))
	switch key.String() {
	case "esc":
		m.search = nil
	case "up":
		m.search.cursor = max(0, m.search.cursor-1)
	case "down":
		m.search.cursor = min(max(0, len(results)-1), m.search.cursor+1)
	case "enter":
		if len(results) > 0 {
			m.openTaskEditor(results[m.search.cursor])
			m.editor.readOnly = true
			m.detailScroll = 0
		}
	default:
		if m.search.focusAll {
			return m, nil
		}
		before := m.search.input.Value()
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(key)
		if before != m.search.input.Value() {
			m.search.cursor = 0
		}
		return m, cmd
	}
	return m, nil
}

func (m model) searchView() string {
	width := min(64, m.width-8)
	results := m.searchResults()
	visible := max(1, (m.height-12)/3)
	cursor := max(0, min(m.search.cursor, len(results)-1))
	start := max(0, cursor-visible+1)
	field := m.search.input
	field.Width = width - 6
	checkbox := "[ ] all tasks"
	if m.search.allTasks {
		checkbox = "[x] all tasks"
	}
	checkboxStyle := dim
	if m.search.focusAll {
		checkboxStyle = selected
	}
	lines := []string{titleStyle.Render("Search tasks"), field.View(), checkboxStyle.Render(checkbox), dim.Render(fmt.Sprintf("%d results", len(results))), ""}
	for i := start; i < min(len(results), start+visible); i++ {
		lines = append(lines, m.taskListItem(results[i], width-4, !m.search.focusAll && i == cursor), "")
	}
	if len(results) == 0 {
		lines = append(lines, dim.Render(labels.Empty))
	}
	bodyHeight := max(1, m.height-8)
	body := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(strings.Join(lines, "\n"))
	lines = []string{body}
	lines = append(lines, dim.Render("tab focus  space toggle  ↑/↓ select  enter view  esc close"))
	if m.err != "" {
		lines = append(lines, m.err)
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(width-2).Height(max(1, m.height-6)).Render(strings.Join(lines, "\n")))
}

func (m *model) move(n int) {
	if m.focusSide {
		m.side += n
		if m.side < 0 {
			m.side = m.sideCount() - 1
		}
		if m.side >= m.sideCount() {
			m.side = 0
		}
		return
	}
	if m.page == pageCalendar {
		if m.calendarTasks {
			m.cursor += n
			m.cursor = max(0, min(m.cursor, len(m.tasksDueOn(m.calDate))-1))
		} else if m.week {
			m.moveCalendarDays(2 * n)
		} else {
			m.moveCalendarDays(7 * n)
		}
		return
	}
	m.cursor += n
	m.clamp()
	m.detailScroll = 0
}

func (m *model) scrollTaskDetail(key string) {
	t, ok := m.selectedTask()
	if !ok {
		return
	}
	width := min(64, max(38, m.width/3)) - 4
	lines := m.taskDetailLines(t, width)
	visible := max(1, m.height-5) // one line is reserved for the scroll indicator
	last := max(0, len(lines)-visible)
	page := max(1, visible-1) // keep one line of context between pages
	switch key {
	case "home":
		m.detailScroll = 0
	case "end":
		m.detailScroll = last
	case "pgup":
		m.detailScroll = max(0, m.detailScroll-page)
	case "pgdown":
		m.detailScroll = min(last, m.detailScroll+page)
	}
}
func (m *model) moveCalendarHorizontal(step int) {
	day := (int(m.calDate.Weekday()) + 6) % 7
	columns, days := 7, 7
	if m.week {
		columns = 2
		if m.workingWeek {
			days = 5
		}
	}
	if step < 0 && day%columns == 0 {
		m.focusSide = true
		return
	}
	if step > 0 && (day%columns == columns-1 || day == days-1) {
		return
	}
	m.moveCalendarDays(step)
}

func (m *model) moveCalendarDays(n int) {
	m.calendarTasks = false
	m.cursor = 0
	step := 1
	if n < 0 {
		step = -1
	}
	distance := n
	if distance < 0 {
		distance = -distance
	}
	for moved := 0; moved < distance; {
		m.calDate = m.calDate.AddDate(0, 0, step)
		if !m.workingWeek || m.calDate.Weekday() != time.Saturday && m.calDate.Weekday() != time.Sunday {
			moved++
		}
	}
}
func (m *model) moveCalendar(n int) {
	m.calendarTasks = false
	m.cursor = 0
	if m.week {
		m.calDate = m.calDate.AddDate(0, 0, 7*n)
	} else {
		m.calDate = m.calDate.AddDate(0, n, 0)
	}
}
func (m *model) clamp() {
	l := m.currentLen()
	if l == 0 {
		m.cursor = 0
	} else if m.cursor >= l {
		m.cursor = l - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}
func (m model) sideCount() int { return sideFirstProject + len(m.projects) + 1 + len(m.tags) }
func (m *model) chooseSide() tea.Cmd {
	m.cursor = 0
	if m.side == sideStatistics {
		m.page, m.lastFilter, m.focusSide = pageStatistics, "statistics", false
		m.statsCursor = 0
		m.statsPeriodCursor = m.statsPeriod
		m.statsTasksFocus = false
		return m.saveStateCmd()
	}
	if m.side == sideDashboard {
		m.page = pageDashboard
		m.lastFilter = "dashboard"
		m.focusSide = false
		return m.saveStateCmd()
	}
	if m.side == sideAllFilters {
		m.page, m.lastFilter = pageAll, "all"
		return m.saveStateCmd()
	}
	if m.side >= sideToday && m.side <= sideDone {
		m.page = page(m.side - sideToday)
		m.lastFilter = []string{"today", "upcoming", "unplanned", "done"}[m.side-sideToday]
		if m.page == pageDone {
			m.showCompleted = true
			m.menu, m.menuIndex = "done-period", 0
		}
		return m.saveStateCmd()
	}
	if m.side == sideCalendar || m.side == sideTags {
		m.page = page(m.side - sideToday)
		m.focusSide = false
		return nil
	}
	if m.side == sideAllProjects {
		m.selectedProjectUID = ""
		return m.saveStateCmd()
	}
	if i := m.side - sideFirstProject - len(m.projects); i >= 0 && i <= len(m.tags) {
		m.tagFilters = map[string]bool{}
		if i > 0 {
			m.tagFilters[strings.ToLower(m.tags[i-1].Name)] = true
		}
		if m.page == pageDashboard || m.page == pageCalendar || m.page == pageTags {
			m.page = pageFromFilter(m.lastFilter)
			if m.page == pageDashboard {
				m.page, m.lastFilter = pageAll, "all"
			}
		}
		return m.saveStateCmd()
	}
	p := m.projects[m.side-sideFirstProject]
	m.selectedProjectUID = p.UID
	return m.saveStateCmd()
}

func (m model) filteredTasks() []task {
	now := time.Now()
	y, mo, d := now.Date()
	start := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)
	var out []task
	for _, t := range m.tasks {
		if m.page == pageDone {
			if !t.completed() {
				continue
			}
		} else if m.statusFilter == "" && t.completed() != m.showCompleted || m.statusFilter != "" && statusName(t.Status) != m.statusFilter {
			continue
		}
		if !m.matchesSelectedProject(t) {
			continue
		}
		if m.priorityFilter != "" && priorityName(t.Priority) != m.priorityFilter {
			continue
		}
		if len(m.tagFilters) > 0 {
			matchTag := false
			for _, tag := range t.Tags {
				matchTag = matchTag || m.tagFilters[strings.ToLower(tag.Name)]
			}
			if !matchTag {
				continue
			}
		}
		due, hasDue := t.due()
		match := false
		switch m.page {
		case pageToday:
			match = hasDue && due.Before(end)
		case pageUpcoming:
			match = hasDue && !due.Before(end)
		case pageUnplanned:
			match = !hasDue
		case pageDone:
			completed, ok := t.completedDate()
			match = ok && !completed.Before(m.doneFrom) && completed.Before(m.doneTo)
		default:
			match = true
		}
		if match {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if m.page == pageAll {
			a, aDue := out[i].due()
			b, bDue := out[j].due()
			if aDue != bDue {
				return aDue
			}
			if aDue && !a.Equal(b) {
				return a.Before(b)
			}
			order := map[string]int{"low": 0, "medium": 1, "high": 2}
			return order[priorityName(out[i].Priority)] > order[priorityName(out[j].Priority)]
		}
		less := false
		switch m.sortBy {
		case "name":
			less = strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		case "priority":
			order := map[string]int{"low": 0, "medium": 1, "high": 2}
			less = order[priorityName(out[i].Priority)] < order[priorityName(out[j].Priority)]
		case "tag":
			first := func(t task) string {
				if len(t.Tags) == 0 {
					return "~"
				}
				names := make([]string, len(t.Tags))
				for i, tag := range t.Tags {
					names[i] = strings.ToLower(tag.Name)
				}
				slices.Sort(names)
				return names[0]
			}
			less = first(out[i]) < first(out[j])
		default:
			a, aDue := out[i].due()
			b, bDue := out[j].due()
			if aDue != bDue {
				less = aDue
			} else {
				less = aDue && a.Before(b)
			}
		}
		return less
	})
	if m.sortDesc && m.page != pageAll {
		slices.Reverse(out)
	}
	return out
}

func taskDateGroup(t task, today time.Time) int {
	due, ok := t.due()
	if !ok {
		return 3
	}
	if due.Before(today) {
		return 0
	}
	if due.Before(today.AddDate(0, 0, 1)) {
		return 1
	}
	return 2
}

func (m model) currentLen() int {
	if m.page == pageDashboard || m.page == pageStatistics {
		return 0
	}
	if m.page == pageTags {
		return len(m.tags)
	}
	if m.page == pageCalendar {
		if m.calendarTasks {
			return len(m.tasksDueOn(m.calDate))
		}
		return 0
	}
	return len(m.filteredTasks())
}
func (m model) selectedTask() (task, bool) {
	if m.page == pageCalendar && m.calendarTasks {
		ts := m.tasksDueOn(m.calDate)
		if m.cursor < len(ts) {
			return ts[m.cursor], true
		}
		return task{}, false
	}
	ts := m.filteredTasks()
	if m.page == pageStatistics || m.page == pageDashboard || m.page == pageTags || m.page == pageCalendar || m.cursor >= len(ts) {
		return task{}, false
	}
	return ts[m.cursor], true
}
func (m model) selectedExists() bool {
	if _, ok := m.focusedProject(); ok {
		return true
	}
	if m.page == pageTags {
		return m.cursor < len(m.tags)
	}
	_, ok := m.selectedTask()
	return ok
}

func (m model) selectedProject() *project {
	for i := range m.projects {
		if m.projects[i].UID == m.selectedProjectUID {
			return &m.projects[i]
		}
	}
	return nil
}

func (m model) focusedProject() (project, bool) {
	if m.focusSide && m.side >= sideFirstProject && m.side-sideFirstProject < len(m.projects) {
		return m.projects[m.side-sideFirstProject], true
	}
	return project{}, false
}

func (m model) matchesSelectedProject(t task) bool {
	p := m.selectedProject()
	if p == nil {
		return true
	}
	return t.ProjectID != 0 && t.ProjectID == p.ID || t.ProjectUID != "" && t.ProjectUID == p.UID || t.Project != nil && t.Project.UID != "" && t.Project.UID == p.UID
}

func input(value, placeholder string) textinput.Model {
	i := textinput.New()
	i.SetValue(value)
	i.Placeholder = placeholder
	i.CharLimit = 200
	i.Width = 48
	return i
}
func note(value, placeholder string) textarea.Model {
	t := textarea.New()
	t.SetValue(value)
	t.Placeholder = placeholder
	t.SetWidth(58)
	t.SetHeight(10)
	return t
}

func (m *model) openNew() {
	m.editor = &editor{kind: editTask, create: true, fields: []textinput.Model{input("", "Name"), input("Medium", "Priority"), input("Not started", "Status"), input("", "Due date"), input("", "Project"), input("", "Tags, comma separated")}, note: note("", "Markdown description")}
	if m.selectedProjectUID != "" {
		if p := m.selectedProject(); p != nil {
			m.editor.fields[4].SetValue(p.Name)
		}
	}
	m.editor.fields[0].Focus()
}

func (m *model) openEdit() {
	if p, ok := m.focusedProject(); ok {
		m.editor = &editor{kind: editProject, project: p, fields: []textinput.Model{input(p.Name, "Name"), input(p.Color, "#RRGGBB"), input(p.Status, "Status")}, note: note(p.Description, "Description")}
	} else if m.page == pageTags && m.cursor < len(m.tags) {
		t := m.tags[m.cursor]
		m.editor = &editor{kind: editTag, tag: t, fields: []textinput.Model{input(t.Name, "Name"), input(t.Color, "#RRGGBB")}}
	} else if t, ok := m.selectedTask(); ok {
		m.openTaskEditor(t)
	}
	if m.editor != nil {
		m.editor.fields[0].Focus()
	}
}

func (m *model) openTaskEditor(t task) {
	var due, projectName string
	if d, ok := t.due(); ok {
		due = d.Format("2006-01-02")
	}
	if t.Project != nil {
		projectName = t.Project.Name
	} else {
		for _, p := range m.projects {
			if t.ProjectID != 0 && p.ID == t.ProjectID || t.ProjectUID != "" && p.UID == t.ProjectUID {
				projectName = p.Name
			}
		}
	}
	names := make([]string, len(t.Tags))
	for i, v := range t.Tags {
		names[i] = v.Name
	}
	m.editor = &editor{kind: editTask, task: t, fields: []textinput.Model{input(t.Name, "Name"), input(priorityLabel(t.Priority), "Priority"), input(statusLabel(t.Status), "Status"), input(due, "Due date"), input(projectName, "Project"), input(strings.Join(names, ", "), "Tags")}, note: note(t.Note, "Markdown description")}
	m.editor.fields[0].Focus()
}

func (m model) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "n":
		m.confirm = false
	case "y", "enter":
		m.loading = true
		m.confirm = false
		if p, ok := m.focusedProject(); ok {
			return m, action(func() error { return m.api.deleteProject(p) })
		}
		if m.page == pageTags && m.cursor < len(m.tags) {
			t := m.tags[m.cursor]
			return m, action(func() error { return m.api.deleteTag(t) })
		}
		if t, ok := m.selectedTask(); ok {
			return m, action(func() error { return m.api.deleteTask(t) })
		}
	}
	return m, nil
}

func (m model) updateEditor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.editor
	if e.readOnly {
		switch key.String() {
		case "e":
			e.readOnly = false
			return m, e.fields[0].Focus()
		case "esc":
			m.editor = nil
			m.detailScroll = 0
		case "up", "pgup":
			m.detailScroll = max(0, m.detailScroll-1)
		case "down", "pgdown":
			m.detailScroll++
		}
		return m, nil
	}
	if e.calendar {
		return m.updateMiniCalendar(key)
	}
	if e.colorPicker {
		return m.updateColorPicker(key)
	}
	if e.choice != 0 {
		return m.updateTaskChoice(key)
	}
	switch key.String() {
	case "esc":
		if m.search != nil {
			m.openTaskEditor(e.task)
			m.editor.readOnly = true
		} else {
			m.editor = nil
		}
		return m, nil
	case "ctrl+s":
		return m.saveEditor()
	case "ctrl+d":
		if e.kind == editTask {
			d, err := time.Parse("2006-01-02", e.fields[3].Value())
			if err != nil {
				d = time.Now()
			}
			e.date, e.calendar = d, true
		}
		return m, nil
	case "enter", "ctrl+p":
		if e.kind == editTask && e.focus < len(e.fields) {
			switch e.focus {
			case 1:
				e.choice, e.choiceIndex = 3, max(0, slices.Index([]string{"low", "medium", "high"}, priorityName(e.fields[1].Value())))
			case 2:
				e.choice, e.choiceIndex = 4, max(0, slices.Index(taskStatuses, statusName(e.fields[2].Value())))
			case 3:
				d, err := time.Parse("2006-01-02", e.fields[3].Value())
				if err != nil {
					d = time.Now()
				}
				e.date, e.calendar = d, true
			case 4:
				e.choice, e.choiceIndex = 1, 0
				for i, p := range m.projects {
					if strings.EqualFold(p.Name, e.fields[4].Value()) {
						e.choiceIndex = i + 1
					}
				}
			case 5:
				e.choice, e.choiceIndex = 2, 0
			}
			return m, nil
		}
		if (e.kind == editTag || e.kind == editProject) && e.focus == 1 {
			e.colorIndex = 0
			for i, color := range colorPalette {
				if strings.EqualFold(color, e.fields[1].Value()) {
					e.colorIndex = i + 1
				}
			}
			e.colorPicker = true
			return m, nil
		}
	case "tab", "shift+tab", "up", "down":
		if e.focus == len(e.fields) && (key.String() == "up" || key.String() == "down") {
			if key.String() == "down" && e.note.Line() == e.note.LineCount()-1 {
				return m, nil
			}
			break
		}
		if e.focus == len(e.fields) {
			e.note.Blur()
		} else {
			e.fields[e.focus].Blur()
		}
		if key.String() == "tab" || key.String() == "down" {
			e.focus++
		} else {
			e.focus--
		}
		max := len(e.fields)
		if e.kind == editTag || e.kind == editConfig {
			max--
		}
		if e.focus > max {
			e.focus = 0
		}
		if e.focus < 0 {
			e.focus = max
		}
		if e.focus == len(e.fields) {
			e.note.Focus()
		} else {
			e.fields[e.focus].Focus()
		}
		return m, nil
	case "pgup", "pgdown":
		if e.focus == len(e.fields) {
			move := tea.KeyUp
			if key.String() == "pgdown" {
				move = tea.KeyDown
			}
			for range max(1, e.note.Height()-1) {
				e.note, _ = e.note.Update(tea.KeyMsg{Type: move})
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	if e.focus == len(e.fields) {
		if e.kind == editTask {
			editorW := min(64, max(38, m.width/3))
			if e.create || m.search != nil {
				editorW = 64
			}
			e.note.SetWidth(max(10, editorW-6))
			e.note.SetHeight(max(3, m.height-26))
			if e.create || m.search != nil {
				e.note.SetHeight(max(3, m.height-22))
			}
		}
		e.note, cmd = e.note.Update(key)
	} else if e.kind == editTask && e.focus >= 1 && e.focus <= 5 {
		return m, nil
	} else {
		e.fields[e.focus], cmd = e.fields[e.focus].Update(key)
	}
	return m, cmd
}

func (m model) updateTaskChoice(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.editor
	count := len(m.projects) + 1
	if e.choice == 2 {
		count = len(m.tags)
	} else if e.choice == 3 {
		count = 3
	} else if e.choice == 4 {
		count = len(taskStatuses)
	}
	switch key.String() {
	case "esc", "tab", "shift+tab":
		e.choice = 0
	case "up", "k":
		if count > 0 {
			e.choiceIndex = (e.choiceIndex - 1 + count) % count
		}
	case "down", "j":
		if count > 0 {
			e.choiceIndex = (e.choiceIndex + 1) % count
		}
	case "enter", " ":
		if e.choice == 4 {
			e.fields[2].SetValue(statusLabel(taskStatuses[e.choiceIndex]))
			e.choice = 0
		} else if e.choice == 3 {
			e.fields[1].SetValue(priorityLabel([]string{"low", "medium", "high"}[e.choiceIndex]))
			e.choice = 0
		} else if e.choice == 1 {
			if e.choiceIndex == 0 {
				e.fields[4].SetValue("")
			} else {
				e.fields[4].SetValue(m.projects[e.choiceIndex-1].Name)
			}
			e.choice = 0
		} else if count > 0 {
			name := m.tags[e.choiceIndex].Name
			selected := selectedNames(e.fields[5].Value())
			if selected[strings.ToLower(name)] {
				delete(selected, strings.ToLower(name))
			} else {
				selected[strings.ToLower(name)] = true
			}
			var names []string
			for _, tag := range m.tags {
				if selected[strings.ToLower(tag.Name)] {
					names = append(names, tag.Name)
				}
			}
			e.fields[5].SetValue(strings.Join(names, ", "))
		}
	}
	return m, nil
}

func selectedNames(value string) map[string]bool {
	selected := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			selected[strings.ToLower(name)] = true
		}
	}
	return selected
}

func (m model) updateMenu(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.menu == "done-calendar" {
		switch key.String() {
		case "esc":
			m.menu, m.donePicking = "done-period", 0
		case "left", "h":
			m.doneDate = m.doneDate.AddDate(0, 0, -1)
		case "right", "l":
			m.doneDate = m.doneDate.AddDate(0, 0, 1)
		case "up", "k":
			m.doneDate = m.doneDate.AddDate(0, 0, -7)
		case "down", "j":
			m.doneDate = m.doneDate.AddDate(0, 0, 7)
		case "[":
			m.doneDate = m.doneDate.AddDate(0, -1, 0)
		case "]":
			m.doneDate = m.doneDate.AddDate(0, 1, 0)
		case "enter", " ":
			if m.donePicking == 0 {
				m.doneFrom, m.donePicking = dayStart(m.doneDate), 1
			} else {
				m.doneTo = dayStart(m.doneDate).AddDate(0, 0, 1)
				if m.doneTo.Before(m.doneFrom) {
					m.doneFrom, m.doneTo = m.doneTo.AddDate(0, 0, -1), m.doneFrom.AddDate(0, 0, 1)
				}
				m.menu, m.donePicking, m.cursor = "", 0, 0
			}
		}
		return m, nil
	}
	if m.menu == "done-period" {
		if key.String() == "esc" {
			m.menu = ""
			return m, nil
		}
		if key.String() == "up" || key.String() == "k" {
			m.menuIndex = (m.menuIndex + 4) % 5
		}
		if key.String() == "down" || key.String() == "j" {
			m.menuIndex = (m.menuIndex + 1) % 5
		}
		if key.String() == "enter" || key.String() == " " {
			now := dayStart(time.Now())
			weekStart := now.AddDate(0, 0, -(int(now.Weekday())+6)%7)
			monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			switch m.menuIndex {
			case 0:
				m.doneFrom, m.doneTo = now, now.AddDate(0, 0, 1)
			case 1:
				m.doneFrom, m.doneTo = now.AddDate(0, 0, -1), now
			case 2:
				m.doneFrom, m.doneTo = weekStart.AddDate(0, 0, -7), weekStart
			case 3:
				m.doneFrom, m.doneTo = monthStart.AddDate(0, -1, 0), monthStart
			case 4:
				m.menu, m.doneDate, m.donePicking = "done-calendar", now, 0
				return m, nil
			}
			m.menu, m.cursor = "", 0
		}
		return m, nil
	}
	count := 4
	if m.menu == "sort-direction" {
		count = 2
	}
	if m.menu == "filter" {
		count = 13 + len(m.tags)
	}
	switch key.String() {
	case "esc":
		if m.menu == "sort-direction" {
			m.menu, m.menuIndex = "sort", slices.Index([]string{"date", "name", "priority", "tag"}, m.sortBy)
		} else {
			m.menu = ""
		}
	case "up", "k":
		m.menuIndex = (m.menuIndex - 1 + count) % count
	case "down", "j":
		m.menuIndex = (m.menuIndex + 1) % count
	case "enter", " ":
		if m.menu == "sort" {
			m.sortBy = []string{"date", "name", "priority", "tag"}[m.menuIndex]
			m.menu, m.menuIndex = "sort-direction", 0
			if m.sortDesc {
				m.menuIndex = 1
			}
			return m, nil
		}
		if m.menu == "sort-direction" {
			m.sortDesc = m.menuIndex == 1
			m.menu, m.cursor = "", 0
			return m, m.saveStateCmd()
		}
		switch {
		case m.menuIndex < 4:
			m.priorityFilter = []string{"", "low", "medium", "high"}[m.menuIndex]
		case m.menuIndex < 11:
			m.statusFilter = append([]string{""}, taskStatuses...)[m.menuIndex-4]
		case m.menuIndex < 11+len(m.tags):
			name := strings.ToLower(m.tags[m.menuIndex-11].Name)
			if m.tagFilters[name] {
				delete(m.tagFilters, name)
			} else {
				m.tagFilters[name] = true
			}
		case m.menuIndex == 11+len(m.tags):
			m.priorityFilter, m.statusFilter, m.tagFilters = "", "", map[string]bool{}
		default:
			m.menu, m.cursor = "", 0
		}
		return m, m.saveStateCmd()
	}
	return m, nil
}

func (m model) menuView() string {
	if m.menu == "done-calendar" {
		prompt := "Choose start date"
		if m.donePicking == 1 {
			prompt = "Choose end date"
		}
		return titleStyle.Render(prompt) + "\n\n" + calendarGrid(m.doneDate) + "\n\n" + dim.Render("arrows move  [ ] month  enter choose  esc back")
	}
	if m.menu == "done-period" {
		var lines []string
		for i, value := range []string{"Today", "Yesterday", "Last week", "Last month", "Custom period"} {
			mark := "  "
			if i == m.menuIndex {
				mark = "> "
			}
			lines = append(lines, mark+value)
		}
		return titleStyle.Render("Done period") + "\n\n" + strings.Join(lines, "\n") + "\n\n" + dim.Render("enter choose  esc close")
	}
	if m.menu == "sort" {
		var lines []string
		for i, value := range []string{"Due date", "Name", "Priority", "Tag"} {
			key := []string{"date", "name", "priority", "tag"}[i]
			mark := "  "
			if i == m.menuIndex {
				mark = "> "
			}
			active := ""
			if m.sortBy == key {
				if m.sortDesc {
					active = " ↓"
				} else {
					active = " ↑"
				}
			}
			lines = append(lines, mark+value+active)
		}
		return titleStyle.Render("Sort tasks") + "\n\n" + strings.Join(lines, "\n") + "\n\n" + dim.Render("enter continue  esc close")
	}
	if m.menu == "sort-direction" {
		var lines []string
		for i, value := range []string{"Ascending ↑", "Descending ↓"} {
			mark := "  "
			if i == m.menuIndex {
				mark = "> "
			}
			lines = append(lines, mark+value)
		}
		return titleStyle.Render("Sort direction") + "\n\n" + strings.Join(lines, "\n") + "\n\n" + dim.Render("enter apply  esc back")
	}
	priorities := []string{"Any priority", "Low", "Medium", "High"}
	var lines []string
	for i, value := range priorities {
		mark, checked := "  ", "( ) "
		if i == m.menuIndex {
			mark = "> "
		}
		if m.priorityFilter == []string{"", "low", "medium", "high"}[i] {
			checked = "(x) "
		}
		lines = append(lines, mark+checked+value)
	}
	for i, value := range append([]string{"Any status"}, taskStatuses...) {
		mark, checked := "  ", "( ) "
		if 4+i == m.menuIndex {
			mark = "> "
		}
		if m.statusFilter == append([]string{""}, taskStatuses...)[i] {
			checked = "(x) "
		}
		if i > 0 {
			value = statusLabel(value)
		}
		lines = append(lines, mark+checked+value)
	}
	for i, tag := range m.tags {
		index, mark, checked := 11+i, "  ", "[ ] "
		if index == m.menuIndex {
			mark = "> "
		}
		if m.tagFilters[strings.ToLower(tag.Name)] {
			checked = "[x] "
		}
		lines = append(lines, mark+checked+"#"+tag.Name)
	}
	for i, value := range []string{"Clear filters", "Apply"} {
		mark := "  "
		if m.menuIndex == 11+len(m.tags)+i {
			mark = "> "
		}
		lines = append(lines, mark+value)
	}
	return titleStyle.Render("Filter tasks") + "\n\n" + strings.Join(lines, "\n") + "\n\n" + dim.Render("space select  enter apply  esc close")
}

func (m model) updateColorPicker(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.editor
	if e.colorIndex == 0 {
		switch key.String() {
		case "esc":
			e.colorPicker = false
			return m, nil
		case "down", "tab":
			e.colorIndex = 1
			return m, nil
		case "enter":
			if validHexColor(e.fields[1].Value()) {
				e.colorPicker = false
			} else {
				m.err = "Color must use #RRGGBB"
			}
			return m, nil
		}
		var cmd tea.Cmd
		e.fields[1], cmd = e.fields[1].Update(key)
		return m, cmd
	}
	const columns = 6
	switch key.String() {
	case "esc":
		e.colorPicker = false
	case "left", "h":
		if e.colorIndex > 1 {
			e.colorIndex--
		}
	case "right", "l":
		if e.colorIndex < len(colorPalette) {
			e.colorIndex++
		}
	case "up", "k":
		e.colorIndex -= columns
		if e.colorIndex < 1 {
			e.colorIndex = 0
		}
	case "down", "j":
		e.colorIndex = min(len(colorPalette), e.colorIndex+columns)
	case "enter":
		e.fields[1].SetValue(colorPalette[e.colorIndex-1])
		e.colorPicker = false
	}
	return m, nil
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 24)
	return err == nil
}

func (m model) updateMiniCalendar(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.editor
	switch key.String() {
	case "esc":
		e.calendar = false
	case "left", "h":
		e.date = e.date.AddDate(0, 0, -1)
	case "right", "l":
		e.date = e.date.AddDate(0, 0, 1)
	case "up", "k":
		e.date = e.date.AddDate(0, 0, -7)
	case "down", "j":
		e.date = e.date.AddDate(0, 0, 7)
	case "[":
		e.date = e.date.AddDate(0, -1, 0)
	case "]":
		e.date = e.date.AddDate(0, 1, 0)
	case "backspace", "delete":
		e.fields[3].SetValue("")
		if statusName(e.fields[2].Value()) == "planned" {
			e.fields[2].SetValue("Not started")
		}
		e.calendar = false
	case "enter":
		e.fields[3].SetValue(e.date.Format("2006-01-02"))
		if statusName(e.fields[2].Value()) == "not_started" {
			e.fields[2].SetValue("Planned")
		}
		e.calendar = false
	}
	return m, nil
}

func (m model) saveEditor() (tea.Model, tea.Cmd) {
	e := m.editor
	if e.kind == editConfig {
		return m.saveConfigEditor()
	}
	if strings.TrimSpace(e.fields[0].Value()) == "" {
		m.err = "Name is required"
		return m, nil
	}
	m.loading = true
	m.err = ""
	switch e.kind {
	case editTag:
		t := e.tag
		t.Name, t.Color = strings.TrimSpace(e.fields[0].Value()), strings.TrimSpace(e.fields[1].Value())
		return m, action(func() error { return m.api.saveTag(t, e.create) })
	case editProject:
		p := e.project
		p.Name, p.Color, p.Status, p.Description = strings.TrimSpace(e.fields[0].Value()), strings.TrimSpace(e.fields[1].Value()), strings.TrimSpace(e.fields[2].Value()), e.note.Value()
		if p.Status == "" {
			p.Status = "not_started"
		}
		return m, action(func() error { return m.api.saveProject(p, e.create) })
	default:
		t := e.task
		t.Name, t.Priority, t.Status, t.Note, t.DueDate = strings.TrimSpace(e.fields[0].Value()), priorityName(e.fields[1].Value()), statusName(e.fields[2].Value()), e.note.Value(), ""
		if d := strings.TrimSpace(e.fields[3].Value()); d != "" {
			parsed, err := time.Parse("2006-01-02", d)
			if err != nil {
				m.loading = false
				m.err = "Date must be YYYY-MM-DD"
				return m, nil
			}
			t.DueDate = parsed.Format(time.RFC3339)
		}
		if t.DueDate != "" && statusName(t.Status) == "not_started" {
			t.Status = "planned"
		}
		t.ProjectID, t.ProjectUID = 0, ""
		pn := strings.TrimSpace(e.fields[4].Value())
		for _, p := range m.projects {
			if strings.EqualFold(p.Name, pn) {
				t.ProjectID, t.ProjectUID = p.ID, p.UID
				break
			}
		}
		if pn != "" && t.ProjectID == 0 && t.ProjectUID == "" {
			m.loading = false
			m.err = "Unknown project"
			return m, nil
		}
		t.Tags = nil
		for _, name := range strings.Split(e.fields[5].Value(), ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				t.Tags = append(t.Tags, tag{Name: name})
			}
		}
		return m, action(func() error { return m.api.saveTask(t, e.create) })
	}
}

func (m model) View() string {
	if m.width > 0 && (m.width < 80 || m.height < 20) {
		return panel.Render("Terminal too small\nMinimum: 80x20")
	}
	if m.loading && m.editor == nil {
		content := m.loader.View() + " " + titleStyle.Render("Loading…") + "\n\n" + dim.Render("Tasks · Projects · Tags")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			panel.Copy().BorderForeground(blue).Width(36).Align(lipgloss.Center).Padding(1, 2).Render(content))
	}
	if m.search != nil {
		if m.editor != nil {
			return m.editorView()
		}
		return m.searchView()
	}
	if m.editor != nil {
		return m.editorView()
	}
	if m.help {
		return m.helpView()
	}
	if m.confirm {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(42).Render("Delete selected item?\n\n[y/enter] delete   [n/esc] cancel"))
	}
	if m.menu != "" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(52).Render(m.menuView()))
	}
	sideW := 24
	if m.width < 100 {
		sideW = 20
	}
	mainW := m.width - sideW - 1
	rightW := 0
	if m.page != pageStatistics && m.page != pageDashboard && m.page != pageCalendar && m.page != pageTags {
		rightW = min(64, max(38, m.width/3))
		mainW -= rightW
	}
	sidebar := panel.Width(sideW - 2).Height(m.height - 4).Render(m.sidebarView())
	content := panel.Width(mainW - 2).Height(m.height - 4).Render(m.contentView(mainW - 4))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
	if rightW > 0 {
		right := ""
		if !m.focusSide {
			if t, ok := m.selectedTask(); ok {
				right = m.taskDetailView(t, rightW-4, m.height-4)
			}
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, panel.Width(rightW-2).Height(m.height-4).MaxHeight(m.height-2).Render(right))
	}
	footer := m.footerView()
	if m.toast != "" {
		decoration := "✓"
		if m.undoTask != nil && m.toastFrame < 12 {
			decoration = []string{"✦", "✧", "★", "✨"}[m.toastFrame%4]
		}
		message := decoration + "  " + m.toast
		hint := ""
		if m.undoTask != nil {
			hint = "u  undo · restore previous status"
		}
		toast := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true).Render(message) + "\n" + dim.Render(hint)
		popup := panel.Copy().BorderForeground(lipgloss.Color("#22c55e")).Width(44).Align(lipgloss.Center).Render(toast)
		body = overlayCentered(body, popup)
	}
	if m.err != "" {
		footer = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Render(m.err)
	}
	if m.loading {
		footer = m.loader.View() + dim.Render(" Loading tasks, projects & tags…")
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m model) footerView() string {
	completionLabel := "completed"
	if m.showCompleted {
		completionLabel = "active"
	}
	items := [][2]string{{"tab", "focus"}, {"?", "help"}, {"n", "task"}, {"N", "project"}, {"e", "edit"}, {"d", "delete"}, {"space", "complete"}, {"h", completionLabel}, {"s", "sort"}, {"f", "filter"}, {"r", "refresh"}, {"c", "config"}, {"q", "quit"}, {"T", "tag"}}
	if m.width < 120 {
		items = [][2]string{{"tab", "focus"}, {"?", "help"}, {"n", "task"}, {"N", "project"}, {"e", "edit"}, {"d", "delete"}, {"space", "done"}, {"c", "config"}, {"q", "quit"}, {"r", "refresh"}}
	}
	if m.page == pageStatistics {
		items = [][2]string{{"tab", "focus"}, {"?", "help"}, {"←/→", "period"}, {"enter", "select/open"}, {"↑/↓", "tasks"}, {"r", "refresh"}, {"c", "config"}, {"q", "quit"}}
	}
	capStyle := lipgloss.NewStyle().Background(blue).Foreground(lipgloss.Color("#ffffff")).Bold(true).Padding(0, 1)
	columns := make([]string, 0, len(items)/2)
	columnWidth := max(1, (m.width-(len(items)/2-1)*3)/(len(items)/2))
	for i := 0; i < len(items); i += 2 {
		keyWidth := max(lipgloss.Width(items[i][0]), lipgloss.Width(items[i+1][0]))
		rows := make([]string, 2)
		for row := range rows {
			item := items[i+row]
			rows[row] = ansi.Truncate(capStyle.Width(keyWidth+2).Render(item[0])+" "+dim.Render(item[1]), columnWidth, "…")
		}
		columns = append(columns, lipgloss.NewStyle().Width(columnWidth).Render(strings.Join(rows, "\n")))
	}
	separator := dim.Render(" │ \n │ ")
	parts := []string{columns[0]}
	for _, column := range columns[1:] {
		parts = append(parts, separator, column)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m model) sidebarView() string {
	items := []string{"▤  " + labels.Dashboard, "All", "◷  " + labels.Today, "→  " + labels.Upcoming, "○  " + labels.Unplanned, "✓  " + labels.Done, "▦  " + labels.Calendar, "#  " + labels.Tags, "▥  Statistics"}
	var b strings.Builder
	for i, v := range items {
		if i == sideStatistics && m.page == pageStatistics {
			v = "> " + v
		} else if i == sideDashboard && m.page == pageDashboard {
			v = "> " + v
		} else if i >= sideAllFilters && i <= sideDone {
			active := i == sideAllFilters && m.page == pageAll || i >= sideToday && m.page == page(i-sideToday)
			if active {
				v = "> " + v
			} else {
				v = "  " + v
			}
		}
		b.WriteString(m.sideLine(i, v))
		b.WriteByte('\n')
	}
	b.WriteString("\n" + dim.Render(strings.ToUpper(labels.Projects)) + "\n")
	allProjects := "  All"
	if m.selectedProjectUID == "" {
		allProjects = "> All"
	}
	b.WriteString(m.sideLine(sideAllProjects, allProjects) + "\n")
	for i, p := range m.projects {
		prefix := "  "
		if p.UID == m.selectedProjectUID {
			prefix = "> "
		}
		name := prefix + "● " + p.Name
		if p.Color != "" && !(m.focusSide && m.side == i+sideFirstProject) {
			name = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color)).Render(name)
		}
		b.WriteString(m.sideLine(i+sideFirstProject, name))
		b.WriteByte('\n')
	}
	b.WriteString("\n" + dim.Render(strings.ToUpper(labels.Tags)) + "\n")
	allTags := "  All"
	if len(m.tagFilters) == 0 {
		allTags = "> All"
	}
	b.WriteString(m.sideLine(sideFirstProject+len(m.projects), allTags) + "\n")
	for i, t := range m.tags {
		index := sideFirstProject + len(m.projects) + 1 + i
		prefix := "  "
		if m.tagFilters[strings.ToLower(t.Name)] {
			prefix = "> "
		}
		name := prefix + "#" + t.Name
		if t.Color != "" {
			name = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Color)).Render(name)
		}
		b.WriteString(m.sideLine(index, name) + "\n")
	}
	return b.String()
}
func (m model) sideLine(i int, s string) string {
	if m.focusSide && m.side == i {
		return selected.Width(20).Render(s)
	}
	return lipgloss.NewStyle().Width(20).Render(s)
}

func taskGroupHeading(label, background string, width int) string {
	style := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Bold(true).
		Foreground(lipgloss.Color("#111827")).Background(lipgloss.Color(background))
	return "\n" + style.Render(truncate(label, width))
}

func (m model) contentView(width int) string {
	if m.page == pageStatistics {
		return m.statisticsView(width)
	}
	if m.page == pageDashboard {
		return m.dashboardView(width)
	}
	if m.page == pageCalendar {
		return m.calendarView(width)
	}
	if m.page == pageTags {
		return m.tagsView()
	}
	ts := m.filteredTasks()
	heading := "All tasks"
	if m.page <= pageDone {
		heading = []string{labels.Today, labels.Upcoming, labels.Unplanned, labels.Done}[m.page]
	}
	heading = fmt.Sprintf("%s (%d)", heading, len(ts))
	var b strings.Builder
	b.WriteString(titleStyle.Render(heading))
	if p := m.selectedProject(); p != nil {
		style := titleStyle
		if p.Color != "" {
			style = style.Foreground(lipgloss.Color(p.Color))
		}
		b.WriteString(dim.Render("  ·  ") + style.Render(p.Name))
	}
	if m.showCompleted && m.page != pageDone {
		b.WriteString(dim.Render("  · completed only"))
	}
	if m.page == pageAll {
		b.WriteString(dim.Render("  · sort: date, highest priority first"))
	} else if m.sortBy != "" {
		direction := "↑"
		if m.sortDesc {
			direction = "↓"
		}
		b.WriteString(dim.Render("  · sort: " + strings.Title(m.sortBy) + " " + direction))
	}
	if m.priorityFilter != "" || m.statusFilter != "" || len(m.tagFilters) > 0 {
		var filters []string
		if m.priorityFilter != "" {
			filters = append(filters, "priority: "+strings.ToUpper(m.priorityFilter[:1]))
		}
		if m.statusFilter != "" {
			filters = append(filters, "status: "+statusLabel(m.statusFilter))
		}
		for _, tag := range m.tags {
			if m.tagFilters[strings.ToLower(tag.Name)] {
				filters = append(filters, "#"+tag.Name)
			}
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#a855f7")).Bold(true).Render("  · filters: " + strings.Join(filters, ", ")))
	}
	b.WriteString("\n\n")
	if len(ts) == 0 {
		b.WriteString(dim.Render(labels.Empty))
		return b.String()
	}
	listW := width
	visibleCount := max(1, (m.height-9)/3)
	if m.page == pageAll {
		visibleCount = max(1, (m.height-14)/3)
	}
	start := 0
	if m.cursor >= visibleCount {
		start = m.cursor - visibleCount + 1
	}
	end := min(len(ts), start+visibleCount)
	visible := ts[start:end]
	var list strings.Builder
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for i, t := range visible {
		if m.page == pageAll && (i == 0 || taskDateGroup(t, today) != taskDateGroup(visible[i-1], today)) {
			group := taskDateGroup(t, today)
			label := []string{"Late", "Today", "Upcoming", labels.Unplanned}[group]
			background := []string{"#fca5a5", "#fde68a", "#bfdbfe", "#d1d5db"}[group]
			list.WriteString(taskGroupHeading(label, background, listW) + "\n")
		}
		list.WriteString(m.taskListItem(t, listW, !m.focusSide && start+i == m.cursor))
		if i < len(visible)-1 {
			list.WriteString("\n")
			if m.page != pageAll || taskDateGroup(t, today) == taskDateGroup(visible[i+1], today) {
				list.WriteString(dim.Render(strings.Repeat("─", max(1, listW))) + "\n")
			}
		}
	}
	if len(ts) > visibleCount {
		above, below := " ", " "
		if start > 0 {
			above = "↑"
		}
		if end < len(ts) {
			below = "↓"
		}
		body := lipgloss.NewStyle().Height(max(1, m.height-5)).Render(b.String() + list.String())
		return body + "\n" + dim.Render(fmt.Sprintf("%s %d–%d / %d %s", above, start+1, end, len(ts), below))
	}
	return b.String() + list.String()
}

type dashboardStats struct {
	total, inProgress, active, dueToday, overdue, completed int
	unplanned, ageDays, agedTasks                           int
	priorities                                              [3]int
	nextWeek                                                [7]int
	tags                                                    map[string]int
}

func (m model) dashboardStats(now time.Time) dashboardStats {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)
	stats := dashboardStats{tags: map[string]int{}}
	for _, task := range m.tasks {
		if !m.matchesSelectedProject(task) {
			continue
		}
		stats.total++
		if task.completed() {
			stats.completed++
			continue
		}
		stats.active++
		stats.priorities[map[string]int{"low": 0, "medium": 1, "high": 2}[priorityName(task.Priority)]]++
		if created, ok := task.created(); ok && created.Before(now) {
			stats.ageDays += int(now.Sub(created).Hours() / 24)
			stats.agedTasks++
		}
		for _, tag := range task.Tags {
			stats.tags[tag.Name]++
		}
		if task.inProgress() {
			stats.inProgress++
		}
		if due, ok := task.due(); ok {
			due = due.In(now.Location())
			if !due.Before(start) && due.Before(end) {
				stats.dueToday++
			} else if due.Before(start) {
				stats.overdue++
			} else if day := int(due.Sub(start).Hours()/24) - 1; day < len(stats.nextWeek) {
				stats.nextWeek[day]++
			}
		} else {
			stats.unplanned++
		}
	}
	return stats
}

func (m model) dashboardView(width int) string {
	stats := m.dashboardStats(time.Now())
	heading := labels.Dashboard
	if p := m.selectedProject(); p != nil {
		heading += "  ·  " + p.Name
	} else {
		heading += "  ·  All projects"
	}
	cards := []struct {
		label string
		value int
		color string
	}{
		{"Total", stats.total, "#3b82f6"},
		{"In Progress", stats.inProgress, "#f59e0b"},
		{"Active", stats.active, "#06b6d4"},
		{"Due Today", stats.dueToday, "#a855f7"},
		{"Overdue", stats.overdue, "#ef4444"},
		{"Completed", stats.completed, "#22c55e"},
	}
	columns := 3
	cardWidth := max(12, (width-(columns-1)*2)/columns)
	rows := make([]string, 0, (len(cards)+columns-1)/columns)
	for i := 0; i < len(cards); i += columns {
		row := make([]string, 0, columns)
		for _, card := range cards[i:min(i+columns, len(cards))] {
			style := lipgloss.NewStyle().Width(cardWidth-4).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(card.color))
			value := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(card.color)).Render(strconv.Itoa(card.value))
			row = append(row, style.Render(value+"\n"+card.label))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	}
	return titleStyle.Render(heading) + "\n\n" + strings.Join(rows, "\n") + "\n" + m.dashboardCharts(stats, width)
}

func (m model) dashboardCharts(stats dashboardStats, width int) string {
	percent := 0
	if stats.total > 0 {
		percent = stats.completed * 100 / stats.total
	}
	age := "n/a"
	if stats.agedTasks > 0 {
		age = fmt.Sprintf("%dd", stats.ageDays/stats.agedTasks)
	}

	type tagCount struct {
		name  string
		count int
	}
	topTags := make([]tagCount, 0, len(stats.tags))
	for name, count := range stats.tags {
		topTags = append(topTags, tagCount{name, count})
	}
	sort.Slice(topTags, func(i, j int) bool {
		return topTags[i].count > topTags[j].count || topTags[i].count == topTags[j].count && topTags[i].name < topTags[j].name
	})
	topTags = topTags[:min(5, len(topTags))]
	panelWidth := width
	if width >= 90 {
		panelWidth = (width - 1) / 2
	}
	innerWidth := panelWidth - 4
	overview := bar(percent, max(8, innerWidth-17), "#22c55e") + fmt.Sprintf(" %3d%% · %d/%d\n", percent, stats.completed, stats.total) +
		fmt.Sprintf("Unplanned%*d\nAverage age%*s", max(1, innerWidth-9), stats.unplanned, max(1, innerWidth-11), age)

	priorityNames := []string{"Low", "Medium", "High"}
	priorityColors := []string{"#3b82f6", "#f59e0b", "#ef4444"}
	maxPriority := max(stats.priorities[0], max(stats.priorities[1], stats.priorities[2]))
	priorityLines := make([]string, 3)
	for i, count := range stats.priorities {
		priorityLines[i] = fmt.Sprintf("%-7s %s %d", priorityNames[i], countBar(count, maxPriority, max(4, innerWidth-12), priorityColors[i]), count)
	}

	maxTag := 0
	for _, tag := range topTags {
		maxTag = max(maxTag, tag.count)
	}
	tagLines := make([]string, len(topTags))
	for i, tag := range topTags {
		name := ansi.Truncate("#"+tag.name, 16, "…")
		tagLines[i] = fmt.Sprintf("%-16s %s %d", name, countBar(tag.count, maxTag, max(3, innerWidth-21), "#a855f7"), tag.count)
	}
	if len(tagLines) == 0 {
		tagLines = []string{dim.Render("No tags")}
	}

	first := lipgloss.JoinHorizontal(lipgloss.Top,
		dashboardPanel("Completion & planning", overview, panelWidth),
		dashboardPanel("Active by priority", strings.Join(priorityLines, "\n"), panelWidth),
	)
	second := lipgloss.JoinHorizontal(lipgloss.Top,
		dashboardPanel("Next 7 days", weekChart(stats.nextWeek), panelWidth),
		dashboardPanel("Top tags", strings.Join(tagLines, "\n"), panelWidth),
	)
	if width < 90 {
		first = lipgloss.JoinVertical(lipgloss.Left,
			dashboardPanel("Completion & planning", overview, panelWidth),
			dashboardPanel("Active by priority", strings.Join(priorityLines, "\n"), panelWidth),
		)
		second = lipgloss.JoinVertical(lipgloss.Left,
			dashboardPanel("Next 7 days", weekChart(stats.nextWeek), panelWidth),
			dashboardPanel("Top tags", strings.Join(tagLines, "\n"), panelWidth),
		)
	}
	return first + "\n" + second
}

func bar(percent, width int, color string) string {
	filled := percent * width / 100
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("█", filled)) + dim.Render(strings.Repeat("░", width-filled))
}

func countBar(value, largest, width int, color string) string {
	filled := 0
	if largest > 0 {
		filled = value * width / largest
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("█", filled)) + dim.Render(strings.Repeat("░", width-filled))
}

func dashboardPanel(title, content string, width int) string {
	return lipgloss.NewStyle().Width(width-4).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#30363d")).Render(titleStyle.Render(title) + "\n" + content)
}

func weekChart(counts [7]int) string {
	maxCount := 0
	for _, count := range counts {
		maxCount = max(maxCount, count)
	}
	var lines []string
	for row := 2; row > 0; row-- {
		var cells []string
		for _, count := range counts {
			filled := maxCount > 0 && count*2 >= row*maxCount
			cell := " "
			if filled {
				cell = lipgloss.NewStyle().Foreground(lipgloss.Color("#a855f7")).Render("█")
			}
			cells = append(cells, " "+cell+" ")
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	values, days := make([]string, 7), make([]string, 7)
	for i, count := range counts {
		values[i] = fmt.Sprintf("%2d ", count)
		days[i] = time.Now().AddDate(0, 0, i+1).Format("Mon")
	}
	return strings.Join(lines, "\n") + "\n" + strings.Join(values, " ") + "\n" + strings.Join(days, " ")
}

func (m model) taskListItem(t task, width int, active bool) string {
	strike := t.completed() && m.page != pageDone
	prefix := statusIcon(t.Status)
	if prefix != "" {
		prefix += " "
	}
	title := prefix + truncate(t.Name, width-lipgloss.Width(prefix))
	if strike {
		if !active {
			title = lipgloss.NewStyle().Foreground(muted).Faint(true).Strikethrough(true).Render(title)
		}
	}
	var metadata []string
	if m.selectedProjectUID == "" {
		if p := m.taskProject(t); p != nil {
			style := dim
			if active {
				style = lipgloss.NewStyle()
			} else if p.Color != "" {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color))
			}
			metadata = append(metadata, style.Strikethrough(strike && !active).Render("● "+p.Name))
		}
	}
	late, date := "", ""
	for _, tag := range t.Tags {
		if active {
			metadata = append(metadata, "#"+tag.Name)
			continue
		}
		style := dim
		if tag.Color != "" {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(tag.Color))
		}
		metadata = append(metadata, style.Strikethrough(strike && !active).Render("#"+tag.Name))
	}
	if due, ok := t.due(); ok {
		now := time.Now()
		y, month, day := now.Date()
		if !t.completed() && due.Before(time.Date(y, month, day, 0, 0, 0, 0, now.Location())) {
			late = "⚠  late"
			if !active {
				late = lipgloss.NewStyle().Foreground(lipgloss.Color("#dc2626")).Bold(true).Render(late)
			}
		}
		date = "◷ " + due.Format("Jan 02")
		if !active {
			date = dim.Strikethrough(strike).Render(date)
		}
	}
	if late != "" {
		metadata = append(metadata, late)
	}
	priority := map[string]string{"low": "L", "medium": "M", "high": "H"}[priorityName(t.Priority)]
	priorityColor := map[string]string{"L": "#3b82f6", "M": "#f59e0b", "H": "#ef4444"}[priority]
	if !active {
		priority = lipgloss.NewStyle().Foreground(lipgloss.Color(priorityColor)).Bold(true).Strikethrough(strike).Render(priority)
	}
	right := priority
	if date != "" {
		right = date + "  " + priority
	}
	second := ansi.Truncate("  "+strings.Join(metadata, "  "), max(0, width-lipgloss.Width(right)-1), "…")
	second += strings.Repeat(" ", max(1, width-lipgloss.Width(second)-lipgloss.Width(right))) + right
	item := title + "\n" + second
	if active {
		if strike {
			return selected.Width(width).Faint(true).Strikethrough(true).Render(title) + "\n" + selected.Width(width).Faint(true).Strikethrough(true).Render(second)
		}
		return selected.Width(width).Render(item)
	}
	return item
}

func (m model) taskDetail(t task, width int) string {
	var b strings.Builder
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, titleStyle.Render(t.Name)) + "\n")
	b.WriteString(dim.Render(strings.Repeat("─", max(1, width))) + "\n")
	if p := m.taskProject(t); p != nil {
		b.WriteString(dim.Render("Project: ") + lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color)).Bold(true).Render(p.Name) + "\n")
	}
	b.WriteString(dim.Render("Priority: ") + lipgloss.NewStyle().Foreground(lipgloss.Color(priorityColor(t.Priority))).Bold(true).Render(priorityLabel(t.Priority)) + "\n")
	status := statusIcon(t.Status)
	if status != "" {
		status += " "
	}
	b.WriteString(dim.Render("Status: ") + lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor(t.Status))).Bold(true).Render(status+statusLabel(t.Status)) + "\n")
	if d, ok := t.due(); ok {
		b.WriteString(dim.Render("Due "+d.Format("Mon, 02 Jan 2006")) + "\n")
	}
	if len(t.Tags) > 0 {
		for _, tag := range t.Tags {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(tag.Color)).Render("#"+tag.Name) + " ")
		}
		b.WriteByte('\n')
	}
	if t.Note != "" {
		b.WriteString(dim.Render(strings.Repeat("─", max(1, width))) + "\n")
		renderer, err := glamour.NewTermRenderer(glamour.WithStylePath("light"), glamour.WithWordWrap(width))
		if err == nil {
			rendered, renderErr := renderer.Render(t.Note)
			if renderErr == nil {
				b.WriteString(rendered)
			} else {
				b.WriteString(t.Note)
			}
		} else {
			b.WriteString(t.Note)
		}
	}
	return b.String()
}

func (m model) taskDetailLines(t task, width int) []string {
	return strings.Split(strings.TrimSuffix(m.taskDetail(t, width), "\n"), "\n")
}

func (m model) taskDetailView(t task, width, height int) string {
	lines := m.taskDetailLines(t, width)
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	visible := max(1, height-1)
	start := min(m.detailScroll, len(lines)-visible)
	percent := start * 100 / (len(lines) - visible)
	arrows := "↑↓"
	if start == 0 {
		arrows = " ↓"
	} else if start+visible == len(lines) {
		arrows = "↑ "
	}
	label := fmt.Sprintf("%s %3d%%", arrows, percent)
	barWidth := max(0, width-lipgloss.Width(label)-1)
	filled := barWidth * percent / 100
	indicator := strings.Repeat("━", filled) + strings.Repeat("─", barWidth-filled) + " " + label
	return dim.Render(indicator) + "\n" + strings.Join(lines[start:start+visible], "\n")
}

func (m model) taskProject(t task) *project {
	for i := range m.projects {
		if (t.ProjectID != 0 && m.projects[i].ID == t.ProjectID) || (t.ProjectUID != "" && m.projects[i].UID == t.ProjectUID) {
			return &m.projects[i]
		}
	}
	if t.Project != nil {
		return t.Project
	}
	return nil
}

func (m model) tagsView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(labels.Tags) + "\n\n")
	if len(m.tags) == 0 {
		return b.String() + dim.Render(labels.Empty)
	}
	for i, t := range m.tags {
		active := !m.focusSide && i == m.cursor
		dot, color := "●", t.Color
		if !active {
			if t.Color != "" {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color(t.Color)).Render(dot)
			}
			color = dim.Render(color)
		}
		line := fmt.Sprintf("%s  %-28s %s", dot, t.Name, color)
		if active {
			line = selected.Width(46).Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m model) calendarView(width int) string {
	if m.week {
		return m.weekView(width)
	}
	first := time.Date(m.calDate.Year(), m.calDate.Month(), 1, 0, 0, 0, 0, m.calDate.Location())
	start := first.AddDate(0, 0, -(int(first.Weekday())+6)%7)
	cellW := max(7, width/7)
	detail := m.calendarSelection(width)
	cellH := max(3, (m.height-8-lipgloss.Height(detail))/6)
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.calDate.Format("January 2006")) + dim.Render("  arrows day  enter tasks  [/] month  v view") + "\n")
	if detail != "" {
		b.WriteString(detail + "\n")
	}
	for _, d := range []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"} {
		b.WriteString(dim.Width(cellW).Align(lipgloss.Center).Render(d))
	}
	b.WriteByte('\n')
	for row := 0; row < 6; row++ {
		cells := make([]string, 0, 7)
		for col := 0; col < 7; col++ {
			day := start.AddDate(0, 0, row*7+col)
			dayLabel := fmt.Sprintf("%2d", day.Day())
			if !m.focusSide && sameDay(day, m.calDate) {
				dayLabel = selected.Render("▸" + dayLabel)
			}
			text := dayLabel
			tasks := m.tasksDueOn(day)
			if len(tasks) > 0 {
				suffix := ""
				if len(tasks) > 1 {
					suffix = fmt.Sprintf(" + %d", len(tasks)-1)
				}
				nameWidth := cellW - 3 - lipgloss.Width(dayLabel) - len(suffix)
				if nameWidth > 0 {
					text += " " + truncate(tasks[0].Name, nameWidth) + suffix
				} else {
					text += fmt.Sprintf(" + %d", len(tasks))
				}
			}
			st := lipgloss.NewStyle().Width(cellW - 2).MaxWidth(cellW).Height(cellH - 2).MaxHeight(cellH).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#30363d"))
			if day.Month() != m.calDate.Month() {
				st = st.Foreground(muted)
			}
			if !m.focusSide && sameDay(day, m.calDate) {
				st = st.BorderForeground(blue)
			}
			cells = append(cells, st.Render(text))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		if row < 5 {
			b.WriteByte('\n')
		}
	}
	view := b.String()
	if m.calendarTasks {
		return overlayCentered(view, m.calendarTasksPopup(width))
	}
	return view
}
func (m model) weekView(width int) string {
	start := m.calDate.AddDate(0, 0, -(int(m.calDate.Weekday())+6)%7)
	days, title := 7, "Week of "
	if m.workingWeek {
		days, title = 5, "Working week of "
	}
	rows := (days + 1) / 2
	cellW := max(16, width/2)
	detail := m.calendarSelection(width)
	cellH := max(5, (m.height-6-lipgloss.Height(detail))/rows)
	var b strings.Builder
	b.WriteString(titleStyle.Render(title+start.Format("02 January 2006")) + dim.Render("  arrows day  enter tasks  [/] week  v view") + "\n")
	if detail != "" {
		b.WriteString(detail + "\n")
	}
	for row := 0; row < rows; row++ {
		var cells []string
		for col := 0; col < 2; col++ {
			i := row*2 + col
			if i >= days {
				cells = append(cells, lipgloss.NewStyle().Width(cellW).Render(""))
				continue
			}
			d := start.AddDate(0, 0, i)
			dayLabel := titleStyle.Render(d.Format("Monday 02"))
			if !m.focusSide && sameDay(d, m.calDate) {
				dayLabel = selected.Render("▸ " + d.Format("Monday 02"))
			}
			lines := []string{dayLabel}
			tasks := m.tasksDueOn(d)
			available := cellH - 3 // day heading and borders
			visible := available / 2
			if len(tasks) > visible {
				visible = (available - 1) / 2 // reserve the overflow count
			}
			visible = min(len(tasks), visible)
			for _, task := range tasks[:visible] {
				lines = append(lines, m.calendarTaskLines(task, cellW-4, false)...)
			}
			if hidden := len(tasks) - visible; hidden > 0 {
				lines = append(lines, dim.Render(fmt.Sprintf("+ %d", hidden)))
			}
			st := lipgloss.NewStyle().Width(cellW - 2).MaxWidth(cellW).Height(cellH - 2).MaxHeight(cellH).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#30363d"))
			if !m.focusSide && sameDay(d, m.calDate) {
				st = st.BorderForeground(blue)
			}
			cells = append(cells, st.Render(strings.Join(lines, "\n")))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		if row < rows-1 {
			b.WriteByte('\n')
		}
	}
	view := b.String()
	if m.calendarTasks {
		return overlayCentered(view, m.calendarTasksPopup(width))
	}
	return view
}

func (m model) tasksDueOn(day time.Time) []task {
	var out []task
	for _, t := range m.tasks {
		if t.completed() != m.showCompleted {
			continue
		}
		if !m.matchesSelectedProject(t) {
			continue
		}
		if d, ok := t.due(); ok && sameDay(d, day) {
			out = append(out, t)
		}
	}
	return out
}

func (m model) calendarSelection(width int) string {
	return dim.Render(m.calDate.Format("Monday, 02 January 2006"))
}

func (m model) calendarTasksPopup(width int) string {
	tasks := m.tasksDueOn(m.calDate)
	popupW := min(64, max(32, width*2/3))
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.calDate.Format("Monday, 02 January")) + "\n\n")
	visible := max(1, (m.height-12)/3)
	first := max(0, m.cursor-visible+1)
	last := min(len(tasks), first+visible)
	for i := first; i < last; i++ {
		b.WriteString(strings.Join(m.calendarTaskLines(tasks[i], popupW-4, i == m.cursor), "\n"))
		if i < last-1 {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n\n" + dim.Render("↑/↓ task  enter edit  esc close"))
	return panel.Width(popupW - 2).Render(b.String())
}

func overlayCentered(base, popup string) string {
	baseLines, popupLines := strings.Split(base, "\n"), strings.Split(popup, "\n")
	baseW, popupW := lipgloss.Width(base), lipgloss.Width(popup)
	x, y := max(0, (baseW-popupW)/2), max(0, (len(baseLines)-len(popupLines))/2)
	for i, line := range popupLines {
		row := y + i
		if row >= len(baseLines) {
			break
		}
		left := ansi.Cut(baseLines[row], 0, x)
		right := ansi.Cut(baseLines[row], x+popupW, baseW)
		baseLines[row] = left + lipgloss.NewStyle().Width(popupW).Render(line) + right
	}
	return strings.Join(baseLines, "\n")
}

func (m model) calendarTaskLines(t task, width int, active bool) []string {
	lines := strings.Split(m.taskListItem(t, width-2, active), "\n")
	for i := range lines {
		prefix := "  "
		if active && i == 0 {
			prefix = selected.Render("▸ ")
		}
		lines[i] = prefix + lines[i]
	}
	return lines
}

func (m model) editorView() string {
	e := m.editor
	if e.calendar {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(64).Render(m.miniCalendarView()))
	}
	if e.colorPicker {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(64).Render(m.colorPickerView()))
	}
	height := 24
	if e.kind == editTask {
		height = m.height - 4
	}
	if e.kind == editTag || e.kind == editConfig {
		height = 18
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(64).Render(m.editorContent(60, height)))
}

func (m model) editorContent(width, height int) string {
	e := m.editor
	if e.readOnly {
		hint := "e edit  esc results"
		if m.page == pageStatistics && m.search == nil {
			hint = "↑/↓ scroll  e edit  esc statistics"
		}
		return titleStyle.Render("View task") + "\n\n" + lipgloss.NewStyle().Height(height-4).MaxHeight(height-4).Render(m.taskDetailView(e.task, width, height-4)) + "\n" + dim.Render(hint)
	}
	names := map[editorKind][]string{editTask: {"Name", "Priority", "Status", "Due date", "Project", "Tags"}, editProject: {"Name", "Color", "Status"}, editTag: {"Name", "Color"}, editConfig: {"Backend (tududi / open-todo)", "Server URL", "API key"}}[e.kind]
	var b strings.Builder
	actionName := "Edit"
	if e.create {
		actionName = "New"
	}
	kinds := []string{"task", "project", "tag", "configuration"}
	b.WriteString(titleStyle.Render(actionName+" "+kinds[e.kind]) + "\n")
	twoColumns := e.kind == editTask && (e.create || m.search != nil)
	var cells []string
	for i, f := range e.fields {
		f.Width = max(10, width-3)
		if twoColumns && i > 0 {
			f.Width = max(10, (width-4)/2-3)
		}
		if e.kind == editTask {
			switch i {
			case 1:
				f.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(priorityColor(f.Value()))).Bold(true)
			case 2:
				f.Prompt = statusIcon(f.Value())
				if f.Prompt != "" {
					f.Prompt += " "
				}
				f.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor(f.Value()))).Bold(true)
			case 4:
				for _, p := range m.projects {
					if strings.EqualFold(p.Name, f.Value()) {
						f.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color)).Bold(true)
						break
					}
				}
			}
		}
		label, field := dim.Render(names[i]), f.View()
		if i == e.focus {
			label = lipgloss.NewStyle().Foreground(blue).Bold(true).Underline(true).Render(names[i])
		}
		if twoColumns && i > 0 {
			cells = append(cells, lipgloss.NewStyle().Width((width-4)/2).MaxWidth((width-4)/2).Render(label+"\n"+field))
			continue
		}
		b.WriteByte('\n')
		b.WriteString(label + "\n" + field + "\n")
		if e.kind == editTask && e.choice != 0 && i == e.focus {
			b.WriteString(m.taskChoiceView(width) + "\n")
		}
		if e.kind == editTask && i == 5 {
			b.WriteString(m.tagPreview(f.Value()) + "\n")
		}
	}
	if twoColumns {
		for i := 0; i < len(cells); i += 2 {
			row := cells[i]
			if i+1 < len(cells) {
				row = lipgloss.JoinHorizontal(lipgloss.Top, row, "    ", cells[i+1])
			}
			b.WriteString("\n" + row + "\n")
		}
		if e.choice != 0 {
			b.WriteString(m.taskChoiceView(width) + "\n")
		}
		b.WriteString(m.tagPreview(e.fields[5].Value()) + "\n")
	}
	if e.kind != editTag && e.kind != editConfig {
		label := "Description"
		if e.kind == editTask {
			label = "Markdown description"
		}
		note := e.note
		note.SetWidth(max(10, width-2))
		note.SetHeight(max(3, height-23))
		if twoColumns {
			note.SetHeight(max(3, height-18))
		}
		labelView, noteView := dim.Render(label), note.View()
		if e.focus == len(e.fields) {
			labelView = lipgloss.NewStyle().Foreground(blue).Bold(true).Underline(true).Render(label)
		}
		b.WriteString("\n" + labelView + "\n" + noteView + "\n")
	}
	button := lipgloss.NewStyle().Background(lipgloss.Color("#1f6feb")).Foreground(lipgloss.Color("#ffffff")).Bold(true).Padding(0, 1)
	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149")).Render(m.err) + "\n")
	}
	buttons := button.Render("Save  ctrl+s") + "  " + button.Render("Cancel  esc")
	bodyHeight := max(1, height-1)
	body := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(b.String())
	return body + "\n" + lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(buttons)
}

func (m model) taskChoiceView(width int) string {
	e := m.editor
	var options []string
	selected := map[string]bool{}
	if e.choice == 3 {
		options = []string{"low", "medium", "high"}
	} else if e.choice == 4 {
		options = taskStatuses
	} else if e.choice == 1 {
		options = append(options, "No project")
		for _, p := range m.projects {
			options = append(options, p.Name)
		}
	} else {
		selected = selectedNames(e.fields[5].Value())
		for _, tag := range m.tags {
			options = append(options, tag.Name)
		}
	}
	start := max(0, e.choiceIndex-4)
	end := min(len(options), start+6)
	var lines []string
	for i := start; i < end; i++ {
		mark := "  "
		if i == e.choiceIndex {
			mark = "> "
		}
		checked := ""
		if e.choice == 2 {
			checked = "[ ] "
			if selected[strings.ToLower(options[i])] {
				checked = "[x] "
			}
		}
		name := options[i]
		color := ""
		if e.choice == 1 && i > 0 {
			color = m.projects[i-1].Color
		} else if e.choice == 2 {
			color = m.tags[i].Color
		} else if e.choice == 3 {
			name, color = priorityLabel(name), priorityColor(name)
		} else if e.choice == 4 {
			if icon := statusIcon(name); icon != "" {
				name = icon + " " + statusLabel(name)
			} else {
				name = statusLabel(name)
			}
			color = statusColor(options[i])
		}
		name = truncate(name, width-lipgloss.Width(mark+checked))
		if color != "" {
			name = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(name)
		}
		lines = append(lines, mark+checked+name)
	}
	return panel.Width(width - 2).Render(strings.Join(lines, "\n"))
}

func (m model) tagPreview(value string) string {
	var parts []string
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		style := dim
		for _, tag := range m.tags {
			if strings.EqualFold(tag.Name, name) && tag.Color != "" {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(tag.Color))
				break
			}
		}
		parts = append(parts, style.Render("#"+name))
	}
	return strings.Join(parts, "  ")
}

func (m model) colorPickerView() string {
	e := m.editor
	var b strings.Builder
	b.WriteString(titleStyle.Render("Choose a color") + "\n\n")
	custom := " Custom  " + e.fields[1].Value() + " "
	if validHexColor(e.fields[1].Value()) {
		custom = lipgloss.NewStyle().Background(lipgloss.Color(e.fields[1].Value())).Foreground(lipgloss.Color("#ffffff")).Bold(true).Render(custom)
	} else {
		custom = lipgloss.NewStyle().Foreground(muted).Render(custom)
	}
	if e.colorIndex == 0 {
		custom = "› " + custom
	} else {
		custom = "  " + custom
	}
	b.WriteString(custom + "\n\n")
	const columns = 6
	for i, color := range colorPalette {
		swatch := lipgloss.NewStyle().Background(lipgloss.Color(color)).Render("      ")
		if i+1 == e.colorIndex {
			swatch = "[" + swatch + "]"
		} else {
			swatch = " " + swatch + " "
		}
		b.WriteString(swatch + " ")
		if i%columns == columns-1 {
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n" + dim.Render("type custom  ↑/↓ palette  arrows choose  enter apply  esc cancel"))
	return b.String()
}
func (m model) miniCalendarView() string {
	e := m.editor
	first := time.Date(e.date.Year(), e.date.Month(), 1, 0, 0, 0, 0, e.date.Location())
	start := first.AddDate(0, 0, -(int(first.Weekday())+6)%7)
	var b strings.Builder
	b.WriteString(titleStyle.Render(e.date.Format("January 2006")) + "\n Mo Tu We Th Fr Sa Su\n")
	for i := 0; i < 42; i++ {
		d := start.AddDate(0, 0, i)
		s := fmt.Sprintf("%2d", d.Day())
		if sameDay(d, e.date) {
			s = selected.Render(s)
		} else if d.Month() != e.date.Month() {
			s = dim.Render(s)
		}
		b.WriteString(" " + s)
		if i%7 == 6 {
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n←↑↓→ day  [ ] month  enter choose  delete clear")
	return b.String()
}
func (m model) helpView() string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel.Width(60).Render(titleStyle.Render("Keyboard")+"\n\n↑/↓ or j/k  move\n←/→ or tab  switch sidebar/content\ncalendar arrows  select day/task\ncalendar enter   browse day's tasks\nstats ←/→        choose period, enter apply\nstats ↓ / ↑      browse tasks / return to periods\n[/]              previous/next calendar period\nenter            open sidebar item\n/                search filtered tasks\nn                new task\nN                new project\nT                new tag\ne/d              edit/delete\nspace            toggle task completion\nu                undo last completion (6s)\nh                active/completed tasks\nv                calendar view\nr                refresh\nc                configuration\n?                close help\nq                quit"))
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func calendarGrid(date time.Time) string {
	first := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	var b strings.Builder
	b.WriteString(first.Format("January 2006") + "\nMo Tu We Th Fr Sa Su\n")
	for i := 0; i < (int(first.Weekday())+6)%7; i++ {
		b.WriteString("   ")
	}
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		cell := fmt.Sprintf("%2d", d.Day())
		if sameDay(d, date) {
			cell = "[" + strconv.Itoa(d.Day()) + "]"
		} else {
			cell += " "
		}
		b.WriteString(cell)
		if d.Weekday() == time.Sunday {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
func truncate(s string, n int) string {
	if n < 2 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
