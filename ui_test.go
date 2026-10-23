package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestSidebarListsTagsBelowProjects(t *testing.T) {
	m := model{
		projects: []project{{Name: "My project"}},
		tags:     []tag{{Name: "work", Color: "#ff0000"}, {Name: "home"}},
	}
	got := ansi.Strip(m.sidebarView())
	projectIndex := strings.Index(got, "● My project")
	tagsIndex := strings.Index(got, "TAGS")
	workIndex := strings.Index(got, "#work")
	homeIndex := strings.Index(got, "#home")
	if projectIndex < 0 || tagsIndex <= projectIndex || workIndex <= tagsIndex || homeIndex <= workIndex {
		t.Fatalf("tags should appear below projects: %q", got)
	}
	if !strings.Contains(got, "#  Tags") {
		t.Fatalf("tag menu should remain available: %q", got)
	}
}

func TestSidebarTagSelectionFiltersTasks(t *testing.T) {
	for _, p := range []page{pageAll, pageDashboard, pageTags, pageCalendar} {
		m := model{
			page: p, lastFilter: "all", focusSide: true, side: sideFirstProject,
			projects:   []project{{UID: "one"}},
			tags:       []tag{{Name: "Work"}, {Name: "Home"}},
			tagFilters: map[string]bool{"home": true},
			tasks:      []task{{Name: "work task", Tags: []tag{{Name: "work"}}}, {Name: "home task", Tags: []tag{{Name: "Home"}}}},
		}
		m.move(1)
		m.chooseSide()
		if len(m.tagFilters) != 0 || len(m.filteredTasks()) != 2 || m.persistedState().Tags != "" {
			t.Fatal("All tags should clear the tag filter")
		}
		view := ansi.Strip(m.sidebarView())
		if !strings.Contains(view[strings.Index(view, "TAGS"):], "> All") {
			t.Fatal("All should be marked in the tags section")
		}
		m.move(1)
		m.chooseSide()
		if got := m.filteredTasks(); m.page != pageAll || len(got) != 1 || got[0].Name != "work task" {
			t.Fatalf("tag selection from page %d did not filter tasks: %#v", p, got)
		}
		if m.persistedState().Tags != "Work" || !strings.Contains(ansi.Strip(m.sidebarView()), "> #Work") {
			t.Fatal("selected tag should be marked and persisted")
		}
		m.move(1)
		if m.side != m.sideCount()-1 {
			t.Fatal("last tag should be reachable")
		}
		m.move(1)
		if m.side != 0 {
			t.Fatal("navigation should wrap after the last tag")
		}
	}
}

func TestNumericTaskPriorityFromAPI(t *testing.T) {
	var got struct {
		Tasks []task `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(`{"tasks":[{"name":"test","priority":2,"status":0}]}`), &got); err != nil {
		t.Fatal(err)
	}
	if name := priorityName(got.Tasks[0].Priority); name != "high" {
		t.Fatalf("priority = %q, want high", name)
	}
}

func TestTaskViewsAndCompletedToggle(t *testing.T) {
	now := time.Now()
	today := now.Add(-time.Hour).Format(time.RFC3339)
	tomorrow := now.Add(24 * time.Hour).Format(time.RFC3339)
	m := model{page: pageToday, tasks: []task{
		{Name: "overdue", DueDate: today, Status: "not_started"},
		{Name: "later", DueDate: tomorrow, Status: "not_started"},
		{Name: "done", DueDate: today, Status: "done"},
		{Name: "floating", Status: "not_started"},
	}}
	if got := len(m.filteredTasks()); got != 1 {
		t.Fatalf("Today returned %d tasks, want 1", got)
	}
	m.showCompleted = true
	if got := m.filteredTasks(); len(got) != 1 || got[0].Name != "done" {
		t.Fatalf("completed Today tasks = %#v", got)
	}
	m.showCompleted = false
	m.page = pageUpcoming
	if got := len(m.filteredTasks()); got != 1 {
		t.Fatalf("Upcoming returned %d tasks, want 1", got)
	}
	m.page = pageUnplanned
	if got := len(m.filteredTasks()); got != 1 {
		t.Fatalf("Unplanned returned %d tasks, want 1", got)
	}
}

func TestDonePageFiltersCompletionDateAndDoesNotStrikeTitles(t *testing.T) {
	now := time.Now()
	m := model{
		page: pageDone, doneFrom: dayStart(now), doneTo: dayStart(now).AddDate(0, 0, 1),
		tasks: []task{
			{Name: "today", Status: "done", CompletedAt: now.Format(time.RFC3339)},
			{Name: "yesterday", Status: "done", CompletedAt: now.AddDate(0, 0, -1).Format(time.RFC3339)},
			{Name: "active", Status: "not_started", UpdatedAt: now.Format(time.RFC3339)},
		},
	}
	got := m.filteredTasks()
	if len(got) != 1 || got[0].Name != "today" {
		t.Fatalf("Done tasks = %#v", got)
	}
	if view := m.taskListItem(got[0], 60, false); strings.Contains(view, "\x1b[9m") {
		t.Fatalf("Done title is struck: %q", view)
	}
}

func TestDashboardStatsUseSelectedProject(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	m := model{
		selectedProjectUID: "one",
		projects:           []project{{ID: 1, UID: "one"}},
		tasks: []task{
			{ProjectID: 1, Status: "in_progress", Priority: 2, DueDate: "2026-09-24", CreatedAt: "2026-09-20T12:00:00Z", Tags: []tag{{Name: "work"}}},
			{ProjectID: 1, Status: "not_started", DueDate: "2026-09-23"},
			{ProjectID: 1, Status: "done", DueDate: "2026-09-24"},
			{ProjectID: 2, Status: "not_started"},
		},
	}
	got := m.dashboardStats(now)
	if got.total != 3 || got.inProgress != 1 || got.active != 2 || got.dueToday != 1 || got.overdue != 1 || got.completed != 1 {
		t.Fatalf("unexpected totals: %+v", got)
	}
	if got.priorities != [3]int{0, 1, 1} || got.ageDays != 4 || got.tags["work"] != 1 {
		t.Fatalf("dashboard breakdowns = %+v", got)
	}
}

func TestDashboardStatsCharts(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	m := model{tasks: []task{
		{Status: "not_started"},
		{Status: "not_started", DueDate: "2026-09-25", Tags: []tag{{Name: "work"}}},
		{Status: "done"},
	}}
	stats := m.dashboardStats(now)
	if stats.unplanned != 1 || stats.nextWeek[0] != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	view := m.dashboardCharts(stats, 100)
	for _, want := range []string{"Completion & planning", "Active by priority", "Unplanned", "Fri", "#work"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard charts missing %q: %q", want, view)
		}
	}
}

func TestDashboardFitsCommonTerminal(t *testing.T) {
	m := model{page: pageDashboard, width: 160, height: 30}
	for i := 0; i < 8; i++ {
		m.tasks = append(m.tasks, task{Status: "not_started", Priority: i % 3, DueDate: time.Now().AddDate(0, 0, i).Format("2006-01-02"), Tags: []tag{{Name: fmt.Sprintf("tag-%d", i)}}})
	}
	view := m.View()
	if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
		t.Fatalf("dashboard is %dx%d, terminal is %dx%d", lipgloss.Width(view), lipgloss.Height(view), m.width, m.height)
	}
}

func TestShiftTabFromMarkdownEditor(t *testing.T) {
	m := model{page: pageToday}
	m.openNew()
	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()

	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyShiftTab})
	got := updated.(model)
	if want := len(got.editor.fields) - 1; got.editor.focus != want {
		t.Fatalf("focus = %d, want %d", got.editor.focus, want)
	}
}

func TestShiftTabSwitchesMainFocus(t *testing.T) {
	m := model{focusSide: false}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if !updated.(model).focusSide {
		t.Fatal("shift+tab should switch focus back to the sidebar")
	}
}

func TestEnterAddsLineToTaskDescription(t *testing.T) {
	m := model{page: pageToday}
	m.openNew()
	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()
	m.editor.note.SetValue("first line")

	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(model).editor.note.Value(); got != "first line\n" {
		t.Fatalf("description = %q, want a new line", got)
	}
}

func TestPageUpScrollsTaskDescription(t *testing.T) {
	m := model{page: pageToday}
	m.openNew()
	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()
	m.editor.note.SetValue(strings.Repeat("line\n", 20))
	lastLine := m.editor.note.Line()

	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyPgUp})
	if got := updated.(model).editor.note.Line(); got >= lastLine {
		t.Fatalf("cursor line = %d, want before %d", got, lastLine)
	}
}

func TestDownStopsAtEndOfTaskDescription(t *testing.T) {
	m := model{page: pageToday}
	m.openNew()
	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()
	m.editor.note.SetValue("first\nlast")
	before := m.editor.note.LineInfo()

	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyDown})
	after := updated.(model).editor.note.LineInfo()
	if after != before {
		t.Fatalf("cursor moved past the last line: before=%+v after=%+v", before, after)
	}
}

func TestDownDoesNotEnterWrappedContinuationOfLastLine(t *testing.T) {
	m := model{page: pageToday}
	m.openNew()
	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()
	m.editor.note.SetWidth(12)
	m.editor.note.SetValue("a final line that wraps")
	before := m.editor.note.LineInfo()

	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyDown})
	if after := updated.(model).editor.note.LineInfo(); after != before {
		t.Fatalf("cursor entered wrapped continuation: before=%+v after=%+v", before, after)
	}
}

func TestTaskListItemHasTitleMetadataAndDueDate(t *testing.T) {
	task := task{
		Name:    "Ship terminal client",
		DueDate: "2026-09-24T12:00:00Z",
		Tags:    []tag{{Name: "release", Color: "#00ff00"}},
	}
	got := (model{}).taskListItem(task, 60, false)
	if !strings.Contains(got, "Ship terminal client\n") || !strings.Contains(got, "#release") || !strings.Contains(got, "Sep 24") {
		t.Fatalf("unexpected task item: %q", got)
	}
}

func TestSelectedTaskMetadataKeepsHighlight(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := model{projects: []project{{ID: 7, Name: "Work", Color: "#00ff00"}}}
	item := task{Name: "Task", ProjectID: 7, Tags: []tag{{Name: "urgent", Color: "#ff0000"}}, DueDate: "2099-10-26", Priority: "high"}
	got := strings.Split(m.taskListItem(item, 60, true), "\n")[1]
	want := selected.Width(60).Render(ansi.Strip(got))
	if got != want {
		t.Fatalf("selected metadata should have uninterrupted highlight:\ngot  %q\nwant %q", got, want)
	}
}

func TestTaskPrioritySortAndFilters(t *testing.T) {
	m := model{page: pageAll, sortBy: "priority", sortDesc: true, priorityFilter: "high", tagFilters: map[string]bool{"work": true}, tasks: []task{
		{Name: "high work", Priority: "high", Tags: []tag{{Name: "work"}}},
		{Name: "high home", Priority: "high", Tags: []tag{{Name: "home"}}},
		{Name: "low work", Priority: "low", Tags: []tag{{Name: "work"}}},
	}}
	got := m.filteredTasks()
	if len(got) != 1 || got[0].Name != "high work" {
		t.Fatalf("filtered tasks = %#v", got)
	}
	if item := m.taskListItem(got[0], 60, false); !strings.HasSuffix(ansi.Strip(strings.Split(item, "\n")[1]), "H") {
		t.Fatalf("priority is not right aligned: %q", item)
	}
}

func TestSortChoosesCriterionThenDirection(t *testing.T) {
	m := model{menu: "sort"}
	updated, _ := m.updateMenu(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.menu != "sort-direction" || m.sortBy != "date" {
		t.Fatalf("criterion selection = %#v", m)
	}
	m.menuIndex = 1
	updated, _ = m.updateMenu(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(model); got.sortBy != "date" || !got.sortDesc {
		t.Fatalf("descending due date selection = %#v", got.persistedState())
	}

	m = model{menu: "sort", menuIndex: 2}
	updated, _ = m.updateMenu(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	m.menuIndex = 0
	updated, _ = m.updateMenu(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(model); got.sortBy != "priority" || got.sortDesc {
		t.Fatalf("ascending priority selection = %#v", got.persistedState())
	}
}

func TestLateTagIsShownOnlyForOverdueActiveTasks(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	m := model{}
	if got := m.taskListItem(task{Name: "late", DueDate: yesterday}, 60, false); !strings.Contains(got, "⚠  late") || lipgloss.Width(strings.Split(got, "\n")[1]) != 60 {
		t.Fatalf("late tag missing: %q", got)
	}
	if got := m.taskListItem(task{Name: "done", DueDate: yesterday, Status: "done"}, 60, false); strings.Contains(got, "⚠  late") {
		t.Fatalf("completed task marked late: %q", got)
	}
}

func TestCompletedTaskTitleIsStruckAndDimmed(t *testing.T) {
	got := (model{}).taskListItem(task{Name: "finished", Status: "done"}, 60, false)
	want := lipgloss.NewStyle().Foreground(muted).Faint(true).Strikethrough(true).Render("✓ finished")
	if !strings.Contains(got, want) {
		t.Fatalf("completed task is not dimmed and struck: %q", got)
	}
}

func TestSelectedCompletedTaskKeepsFullHighlight(t *testing.T) {
	got := (model{}).taskListItem(task{Name: "finished", Status: "done"}, 60, true)
	for i, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) != 60 {
			t.Fatalf("line %d width = %d, want 60: %q", i, lipgloss.Width(line), line)
		}
	}
}

func TestTaskHeadingCountsCombinedFilters(t *testing.T) {
	m := model{
		page:               pageUnplanned,
		selectedProjectUID: "one",
		projects:           []project{{ID: 1, UID: "one", Name: "One"}},
		tasks:              []task{{Name: "matching", ProjectID: 1}, {Name: "other", ProjectID: 2}},
	}
	if got := m.contentView(80); !strings.Contains(got, "Unplanned (1)") {
		t.Fatalf("filtered task count missing from heading: %q", got)
	}
}

func TestTaskDetailIsAFullHeightRightPanel(t *testing.T) {
	m := model{
		page:   pageUnplanned,
		width:  160,
		height: 30,
		tasks: []task{
			{Name: "short"},
			{Name: "markdown", Note: strings.Repeat("## Heading\n\nParagraph\n\n", 20)},
		},
	}
	m.focusSide = false
	m.cursor = 0
	shortHeight := lipgloss.Height(m.View())
	m.cursor = 1
	markdownHeight := lipgloss.Height(m.View())
	if shortHeight != markdownHeight {
		t.Fatalf("view height changed from %d to %d", shortHeight, markdownHeight)
	}
	if markdownHeight > m.height {
		t.Fatalf("view height = %d, terminal = %d", markdownHeight, m.height)
	}
	m.focusSide = true
	if strings.Contains(ansi.Strip(m.View()), "Paragraph") {
		t.Fatal("detail panel should be empty without a selected task")
	}
}

func TestLongTaskListKeepsCursorVisibleWithoutOverflow(t *testing.T) {
	m := model{page: pageUnplanned, width: 160, height: 30}
	for i := 0; i < 30; i++ {
		m.tasks = append(m.tasks, task{Name: fmt.Sprintf("task %02d", i)})
	}
	m.cursor = len(m.tasks) - 1
	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("view height = %d, terminal height = %d", got, m.height)
	}
	if !strings.Contains(view, "task 29") {
		t.Fatal("selected last task is not visible")
	}
}

func TestProjectSelectionCombinesWithTaskFilterAndAllClears(t *testing.T) {
	now := time.Now().Add(-time.Hour).Format(time.RFC3339)
	m := model{
		page:               pageToday,
		focusSide:          true,
		side:               sideFirstProject,
		selectedProjectUID: "one",
		projects:           []project{{ID: 1, UID: "one"}, {ID: 2, UID: "two"}},
		tasks:              []task{{Name: "one", ProjectID: 1, DueDate: now}, {Name: "two", ProjectID: 2, DueDate: now}},
	}
	if got := m.filteredTasks(); len(got) != 1 || got[0].Name != "one" {
		t.Fatalf("filtered tasks = %#v", got)
	}
	m.chooseSide()
	if m.selectedProjectUID != "one" || !m.focusSide {
		t.Fatal("selecting a project again should keep it selected and focused")
	}
	m.side = sideAllProjects
	m.chooseSide()
	if m.selectedProjectUID != "" || len(m.filteredTasks()) != 2 || !m.focusSide {
		t.Fatal("All projects should clear the project filter and keep sidebar focus")
	}
}

func TestFilterSelectionAndAll(t *testing.T) {
	m := model{
		page:      pageToday,
		focusSide: true,
		side:      sideToday,
		tasks: []task{
			{Name: "dated", DueDate: time.Now().Format("2006-01-02")},
			{Name: "unplanned"},
		},
	}
	if len(m.filteredTasks()) != 1 {
		t.Fatal("today filter should initially be active")
	}
	m.chooseSide()
	if m.page != pageToday || len(m.filteredTasks()) != 1 || !m.focusSide {
		t.Fatal("selecting Today again should keep the filter and sidebar focus")
	}
	m.side = sideAllFilters
	m.chooseSide()
	if m.page != pageAll || len(m.filteredTasks()) != 2 {
		t.Fatal("All filters should show every task")
	}
	if m.persistedState().Filter != "all" {
		t.Fatal("cleared filter should be persisted")
	}
}

func TestDashboardSelectionIsPersisted(t *testing.T) {
	m := model{side: sideDashboard}
	if m.chooseSide() == nil || m.persistedState().Filter != "dashboard" {
		t.Fatal("dashboard selection should be persisted")
	}
	if got := newModel(nil, m.persistedState()); got.page != pageDashboard || got.side != sideDashboard {
		t.Fatalf("dashboard state restored as page=%v side=%v", got.page, got.side)
	}
}

func TestCalendarViewsStayInsideTerminal(t *testing.T) {
	for _, mode := range []struct{ week, working bool }{{}, {week: true}, {week: true, working: true}} {
		m := model{page: pageCalendar, width: 160, height: 30, week: mode.week, workingWeek: mode.working, calDate: time.Now()}
		view := m.View()
		if got := lipgloss.Height(view); got > m.height {
			t.Fatalf("mode=%+v: height = %d, terminal = %d", mode, got, m.height)
		}
		if got := lipgloss.Width(view); got > m.width {
			t.Fatalf("mode=%+v: width = %d, terminal = %d", mode, got, m.width)
		}
	}
}

func TestCalendarNavigationAndTaskBrowsing(t *testing.T) {
	day := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	due := day.Format(time.RFC3339)
	m := model{page: pageCalendar, width: 160, height: 30, calDate: day, tasks: []task{
		{Name: "first task", DueDate: due, Tags: []tag{{Name: "home", Color: "#00ff00"}}},
		{Name: "the complete second task name", DueDate: due, Tags: []tag{{Name: "work", Color: "#ff0000"}}},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.calendarTasks {
		t.Fatal("enter should focus the selected day's tasks")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	popup := m.calendarView(130)
	if m.cursor != 1 || !strings.Contains(popup, "first task") || !strings.Contains(popup, "the complete second task name") || !strings.Contains(popup, "#home") || !strings.Contains(popup, "#work") || !strings.Contains(popup, day.Format("January 2006")) {
		t.Fatal("calendar task navigation did not expose the full task name")
	}

	m.calendarTasks = false
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(model).calDate; !sameDay(got, day.AddDate(0, 0, 1)) {
		t.Fatal("right should select the next calendar day")
	}
}

func TestCalendarTasksReuseListMetadata(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	lines := (model{}).calendarTaskLines(task{
		Name: "late calendar task", Priority: "high", DueDate: yesterday,
		Tags: []tag{{Name: "urgent", Color: "#ff0000"}},
	}, 60, false)
	got := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(got, "#urgent") || !strings.Contains(got, "⚠  late") || !strings.HasSuffix(ansi.Strip(lines[1]), "H") {
		t.Fatalf("calendar task metadata missing: %q", got)
	}
}

func TestEnterEditsCalendarTaskInPopupAndEscapeReturnsToTasks(t *testing.T) {
	day := time.Now()
	m := model{page: pageCalendar, width: 160, height: 30, calDate: day, calendarTasks: true, cursor: 1, tasks: []task{
		{UID: "first", Name: "first task", DueDate: day.Format(time.RFC3339)},
		{UID: "second", Name: "second task", DueDate: day.Format(time.RFC3339)},
	}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.editor == nil || m.editor.task.UID != "second" || !strings.Contains(m.View(), "Edit task") {
		t.Fatal("enter should edit the selected calendar task in a popup")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.editor != nil || !m.calendarTasks || !strings.Contains(m.View(), "second task") {
		t.Fatal("escape should discard editing and return to the day's task list")
	}
}

func TestOpeningCalendarFocusesDayNavigation(t *testing.T) {
	m := model{focusSide: true, side: sideCalendar, calDate: time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.page != pageCalendar || m.focusSide {
		t.Fatal("opening Calendar should focus its day grid")
	}
	day := m.calDate
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(model).calDate; !sameDay(got, day.AddDate(0, 0, 1)) {
		t.Fatal("right should move to the next day immediately after opening Calendar")
	}
}

func TestCalendarViewCycleIncludesWorkingWeek(t *testing.T) {
	m := model{page: pageCalendar}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(model)
	if !m.week || m.workingWeek {
		t.Fatal("first v should select week view")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(model)
	if !m.week || !m.workingWeek {
		t.Fatal("second v should select working-week view")
	}
}

func TestCalendarArrowsMoveBetweenVisibleBlocks(t *testing.T) {
	monday := time.Date(2026, 9, 21, 0, 0, 0, 0, time.Local)
	m := model{page: pageCalendar, calDate: monday}
	m.move(1)
	if want := monday.AddDate(0, 0, 7); !sameDay(m.calDate, want) {
		t.Fatalf("month down = %v, want %v", m.calDate, want)
	}

	m.week, m.calDate = true, monday
	m.move(1)
	if want := monday.AddDate(0, 0, 2); !sameDay(m.calDate, want) {
		t.Fatalf("week down = %v, want %v", m.calDate, want)
	}

	m.workingWeek = true
	m.calDate = time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)
	m.moveCalendarDays(1)
	if m.calDate.Weekday() != time.Monday || m.calDate.Day() != 28 {
		t.Fatalf("working-week right did not cross to Monday: %v", m.calDate)
	}
}

func TestMonthViewShowsSelectedDayClearly(t *testing.T) {
	day := time.Date(2026, 9, 23, 0, 0, 0, 0, time.Local)
	m := model{page: pageCalendar, width: 160, height: 30, calDate: day}
	view := m.calendarView(130)
	if !strings.Contains(view, "Wednesday, 23 September 2026") {
		t.Fatal("month view does not show the selected date")
	}
	want := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(blue).Bold(true).Render("23")
	if !strings.Contains(view, want) {
		t.Fatal("month view does not visibly highlight the selected day")
	}
}

func TestColorPickerAppliesSelectedColor(t *testing.T) {
	m := model{editor: &editor{kind: editTag, focus: 1, fields: []textinput.Model{input("tag", "Name"), input("#3b82f6", "Color")}}}
	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.editor.colorPicker {
		t.Fatal("enter on the color field should open the picker")
	}
	m.editor.colorIndex = 6
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.editor.fields[1].Value(); got != colorPalette[5] {
		t.Fatalf("color = %q, want %q", got, colorPalette[5])
	}
}

func TestColorPickerCustomInputUpdatesLive(t *testing.T) {
	color := input("", "Color")
	color.Focus()
	m := model{editor: &editor{kind: editTag, focus: 1, colorPicker: true, fields: []textinput.Model{input("tag", "Name"), color}}}
	for _, r := range "#123abc" {
		updated, _ := m.updateColorPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	if got := m.editor.fields[1].Value(); got != "#123abc" || !validHexColor(got) {
		t.Fatalf("custom color = %q", got)
	}
	if !strings.Contains(m.colorPickerView(), "#123abc") {
		t.Fatal("custom color preview was not updated")
	}
}

func TestSelectedTagUsesOneContinuousHighlight(t *testing.T) {
	tg := tag{Name: "Documentation", Color: "#15803d"}
	m := model{page: pageTags, tags: []tag{tg}, cursor: 0}
	want := selected.Width(46).Render(fmt.Sprintf("●  %-28s %s", tg.Name, tg.Color))
	if got := m.tagsView(); !strings.Contains(got, want) {
		t.Fatalf("selected tag is not rendered as one highlighted block: %q", got)
	}
}

func TestFooterKeysUseTwoAlignedRows(t *testing.T) {
	for _, width := range []int{80, 100, 120, 160} {
		footer := (model{width: width}).footerView()
		if lipgloss.Height(footer) != 2 || lipgloss.Width(footer) > width {
			t.Fatalf("footer size = %dx%d, terminal width %d", lipgloss.Width(footer), lipgloss.Height(footer), width)
		}
		rows := strings.Split(ansi.Strip(footer), "\n")
		for i, r := range []rune(rows[0]) {
			if r == '│' && []rune(rows[1])[i] != '│' {
				t.Fatal("footer column separators are not aligned")
			}
		}
		if !strings.Contains(rows[0], "n") || !strings.Contains(rows[1], "N") || !strings.Contains(rows[1], "project") {
			t.Fatal("footer must show both creation shortcuts")
		}
	}
}

func TestEnterEditsSelectedTag(t *testing.T) {
	m := model{page: pageTags, tags: []tag{{UID: "tag-1", Name: "Docs"}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)
	if got.editor == nil || got.editor.kind != editTag || got.editor.tag.UID != "tag-1" {
		t.Fatal("enter should edit the selected tag")
	}
}

func TestEditorArrowKeysMoveFocus(t *testing.T) {
	m := model{page: pageToday}
	m.openNew()

	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.editor.focus != 1 {
		t.Fatalf("down focus = %d, want 1", m.editor.focus)
	}

	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if m.editor.focus != 0 {
		t.Fatalf("up focus = %d, want 0", m.editor.focus)
	}

	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyUp})
	if got := updated.(model).editor.focus; got != len(m.editor.fields) {
		t.Fatalf("up in markdown moved focus to %d", got)
	}
}

func TestTaskEditorPickers(t *testing.T) {
	m := model{projects: []project{{Name: "Work", Color: "#ff0000"}}, tags: []tag{{Name: "urgent", Color: "#00ff00"}}}
	m.openNew()

	m.editor.focus = 2
	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	m.editor.choiceIndex = 3
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.editor.fields[2].Value(); got != "Waiting" {
		t.Fatalf("status = %q", got)
	}

	m.editor.focus = 3
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.editor.calendar {
		t.Fatal("enter on due date should open the calendar")
	}
	m.editor.calendar = false

	m.editor.focus = 4
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if want := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("Work"); !strings.Contains(m.taskChoiceView(40), want) {
		t.Fatal("project picker did not use the project color")
	}
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.editor.fields[4].Value(); got != "Work" {
		t.Fatalf("project = %q", got)
	}

	m.editor.focus = 5
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if want := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Render("urgent"); !strings.Contains(m.taskChoiceView(40), want) {
		t.Fatal("tag picker did not use the tag color")
	}
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.editor.fields[5].Value(); got != "urgent" {
		t.Fatalf("tags = %q", got)
	}
}

func TestStatusFilterAndIcons(t *testing.T) {
	m := model{page: pageAll, statusFilter: "waiting", tasks: []task{{Name: "new"}, {Name: "wait", Status: "waiting"}, {Name: "plan", Status: "planned"}, {Name: "done", Status: "done"}}}
	if got := m.filteredTasks(); len(got) != 1 || got[0].Name != "wait" {
		t.Fatalf("filtered tasks = %#v", got)
	}
	for _, task := range m.tasks {
		if got := lipgloss.Width(strings.Split(m.taskListItem(task, 20, true), "\n")[0]); got != 20 {
			t.Fatalf("%s title width = %d, want 20", statusName(task.Status), got)
		}
	}
	if got := ansi.Strip(strings.Split(m.taskListItem(m.tasks[0], 20, false), "\n")[0]); strings.HasPrefix(got, " ") || strings.HasPrefix(got, "📝") {
		t.Fatalf("not-started title has an icon: %q", got)
	}
}

func TestTaskDetailShowsColoredProjectPriorityAndStatus(t *testing.T) {
	m := model{projects: []project{{ID: 7, Name: "Work", Color: "#ff00ff"}}}
	got := m.taskDetail(task{Name: "Task", ProjectID: 7, Project: &project{ID: 7, Name: "Work"}, Priority: "high", Status: "waiting", Note: "Description"}, 40)
	for _, want := range []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff00ff")).Bold(true).Render("Work"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(priorityColor("high"))).Bold(true).Render("High"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor("waiting"))).Bold(true).Render("⏳ Waiting"),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("task detail missing %q: %q", want, got)
		}
	}
	plain := ansi.Strip(got)
	lines := strings.Split(plain, "\n")
	if strings.TrimSpace(lines[0]) != "Task" || lines[1] != strings.Repeat("─", 40) || strings.Count(plain, strings.Repeat("─", 40)) != 2 {
		t.Fatalf("task detail separators missing: %q", plain)
	}
}

func TestTaskDetailScrollsAndResetsOnSelection(t *testing.T) {
	m := model{page: pageAll, width: 160, height: 20, focusSide: false, tasks: []task{
		{Name: "First", Note: strings.Repeat("line\n\n", 40) + "last line"},
		{Name: "Second"},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(model)
	firstPage := m.detailScroll
	if firstPage == 0 || !strings.Contains(ansi.Strip(m.View()), "%") {
		t.Fatal("page down should scroll and show progress")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(model)
	if m.detailScroll <= firstPage {
		t.Fatal("a second page down should continue scrolling")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(model)
	if !strings.Contains(ansi.Strip(m.taskDetailView(m.tasks[0], 49, 17)), "last line") {
		t.Fatal("end should show the bottom of the task description")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.cursor != 1 || m.detailScroll != 0 {
		t.Fatalf("selection = %d, scroll = %d; want 1, 0", m.cursor, m.detailScroll)
	}
}

func TestTaskDetailScrollProgress(t *testing.T) {
	m := model{}
	longTask := task{Name: "Long task", Note: strings.Repeat("line\n\n", 40)}
	const width, height = 40, 16
	last := len(m.taskDetailLines(longTask, width)) - (height - 1)
	for _, offset := range []int{0, last / 2, last, last + 100} {
		m.detailScroll = offset
		view := strings.Split(ansi.Strip(m.taskDetailView(longTask, width, height)), "\n")
		indicator := view[0]
		percent := min(offset, last) * 100 / last
		if len(view) != height || lipgloss.Width(indicator) != width || !strings.HasSuffix(indicator, fmt.Sprintf("%3d%%", percent)) {
			t.Fatalf("offset %d: incorrect progress indicator %q", offset, indicator)
		}
		if strings.Count(indicator, "━") != (width-8)*percent/100 {
			t.Fatalf("offset %d: incorrect progress bar %q", offset, indicator)
		}
	}
	if strings.Contains(ansi.Strip(m.taskDetailView(task{Name: "Short"}, width, height)), "%") {
		t.Fatal("short content should not show a scroll indicator")
	}
}

func TestCalendarKeepsStatusAndPlansDatedTask(t *testing.T) {
	m := model{}
	m.openNew()
	m.editor.fields[2].SetValue("Waiting")
	m.editor.date = time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	updated, _ := m.updateMiniCalendar(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.editor.fields[2].Value() != "Waiting" || m.editor.fields[3].Value() != "2026-09-25" {
		t.Fatalf("fields = status %q, date %q", m.editor.fields[2].Value(), m.editor.fields[3].Value())
	}
	m.editor.fields[2].SetValue("Not started")
	updated, _ = m.updateMiniCalendar(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(model).editor.fields[2].Value(); got != "Planned" {
		t.Fatalf("dated task status = %q", got)
	}
}

func TestLongTaskDescriptionKeepsEditorButtonsVisible(t *testing.T) {
	m := model{page: pageAll, width: 160, height: 30}
	m.openNew()
	m.editor.note.SetValue("first line\n" + strings.Repeat("x", 200) + "TAIL")
	m.editor.focus = len(m.editor.fields)
	m.editor.note.Focus()
	_ = m.View()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = updated.(model)
	view := m.View()
	if !strings.Contains(view, "Save  ctrl+s") || !strings.Contains(view, "Cancel  esc") {
		t.Fatal("fixed editor buttons are not visible")
	}
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("view height = %d, terminal = %d", got, m.height)
	}
	if !strings.Contains(ansi.Strip(view), "TAIL") {
		t.Fatal("end of the active description line is hidden")
	}
	actions := strings.Split(ansi.Strip(m.editorContent(60, 25)), "\n")
	if last := actions[len(actions)-1]; !strings.HasPrefix(last, " ") || !strings.Contains(last, "Save  ctrl+s") {
		t.Fatalf("editor actions are not bottom-right aligned: %q", last)
	}
}

func TestHorizontalArrowsSwitchMainFocus(t *testing.T) {
	m := model{focusSide: true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.focusSide {
		t.Fatal("right should focus the main content")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if !updated.(model).focusSide {
		t.Fatal("left should focus the sidebar")
	}
}

func TestNewTaskRendersInPopup(t *testing.T) {
	m := model{page: pageToday, width: 160, height: 40, tasks: []task{{Name: "Existing task"}}}
	m.openNew()
	view := m.View()
	if view != m.editorView() || !strings.Contains(view, "New task") || strings.Contains(view, "Today") {
		t.Fatalf("new task did not render in the centered popup: %q", view)
	}
	if got := lipgloss.Width(view); got > m.width {
		t.Fatalf("view width = %d, terminal = %d", got, m.width)
	}
}

func TestEditTaskRendersInPopup(t *testing.T) {
	m := model{page: pageAll, width: 160, height: 40, tasks: []task{{Name: "Existing task"}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = updated.(model)
	if m.editor == nil || m.editor.create || m.editor.task.Name != "Existing task" {
		t.Fatal("e should edit the selected task")
	}
	if view := m.View(); view != m.editorView() || !strings.Contains(view, "Edit task") {
		t.Fatal("task editing should render in the centered popup")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(model).editor != nil {
		t.Fatal("escape should close the editor")
	}
}

func TestNewTaskPrefillsSelectedProject(t *testing.T) {
	for _, uid := range []string{"work", "", "missing"} {
		m := model{selectedProjectUID: uid, projects: []project{{ID: 7, UID: "work", Name: "Work"}, {ID: 8, UID: "other", Name: "Other"}}, focusSide: true, side: sideFirstProject + 1}
		m.openNew()
		want := ""
		if uid == "work" {
			want = "Work"
		}
		if got := m.editor.fields[4].Value(); got != want {
			t.Fatalf("selected project %q: got %q, want %q", uid, got, want)
		}
	}
}

func TestNewTaskFieldsUseTwoColumnsWithSpacing(t *testing.T) {
	m := model{width: 160, height: 40}
	m.openNew()
	lines := strings.Split(ansi.Strip(m.editorContent(60, 36)), "\n")
	for _, pair := range [][2]string{{"Priority", "Status"}, {"Due date", "Project"}, {"Tags", ""}} {
		found := false
		for i, line := range lines {
			if strings.Contains(line, pair[0]) && strings.Contains(line, pair[1]) {
				found = true
				if i == 0 || strings.TrimSpace(lines[i-1]) != "" {
					t.Fatalf("missing empty line before %q", line)
				}
				break
			}
		}
		if !found {
			t.Fatalf("missing field row %v", pair)
		}
	}
}

func TestCreationKeysIgnorePageAndFocus(t *testing.T) {
	for _, p := range []page{pageToday, pageTags, pageCalendar, pageDashboard} {
		for _, focus := range []bool{true, false} {
			for key, kind := range map[string]editorKind{"n": editTask, "N": editProject, "T": editTag} {
				m := model{page: p, focusSide: focus, side: sideFirstProject, projects: []project{{Name: "Work"}}}
				updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
				got := updated.(model)
				if got.editor == nil || got.editor.kind != kind || !got.editor.create {
					t.Fatalf("page %d, focus %v, key %s: wrong editor", p, focus, key)
				}
			}
		}
	}
}

func TestTaskListScrollIndicator(t *testing.T) {
	m := model{page: pageAll, width: 160, height: 30}
	for i := 0; i < 30; i++ {
		m.tasks = append(m.tasks, task{Name: fmt.Sprintf("task %02d", i)})
	}
	for _, cursor := range []int{0, 15, 29} {
		m.cursor = cursor
		view := ansi.Strip(m.contentView(70))
		lines := strings.Split(view, "\n")
		if len(lines) != m.height-4 || !strings.Contains(lines[len(lines)-1], "/ 30") {
			t.Fatalf("pager must occupy the last panel line: height %d, last line %q", len(lines), lines[len(lines)-1])
		}
		if !strings.Contains(view, "/ 30") || strings.Contains(view, "↑") != (cursor >= 7) || strings.Contains(view, "↓") != (cursor < 29) {
			t.Fatalf("cursor %d: wrong indicator: %s", cursor, view)
		}
	}
	m.tasks = m.tasks[:2]
	m.cursor = 0
	if strings.Contains(ansi.Strip(m.contentView(70)), "/ 2") {
		t.Fatal("short list should not have a scroll indicator")
	}
}

func TestTaskListShowsColoredProjectBeforeTagsForAllProjects(t *testing.T) {
	p := project{ID: 7, UID: "work", Name: "Work", Color: "#ff00ff"}
	m := model{projects: []project{p}}
	for _, task := range []task{
		{Name: "Task", ProjectID: 7, Tags: []tag{{Name: "urgent"}}},
		{Name: "Task", ProjectUID: "work", Tags: []tag{{Name: "urgent"}}},
		{Name: "Task", Project: &p, Tags: []tag{{Name: "urgent"}}},
	} {
		for _, active := range []bool{true, false} {
			line := strings.Split(m.taskListItem(task, 60, active), "\n")[1]
			want := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color)).Render("● Work")
			if active {
				want = "● Work"
			}
			if !strings.Contains(line, want) || !strings.HasPrefix(strings.TrimSpace(ansi.Strip(line)), "● Work  #urgent") {
				t.Fatalf("missing colored project before tags: %q", line)
			}
		}
		m.selectedProjectUID = p.UID
		if strings.Contains(ansi.Strip(m.taskListItem(task, 60, false)), "● Work") {
			t.Fatal("project should be hidden when filtering by project")
		}
		m.selectedProjectUID = ""
	}
	if strings.Contains(ansi.Strip(m.taskListItem(task{Name: "No project"}, 60, false)), "●") {
		t.Fatal("task without project should have no project label")
	}
}

func TestSidebarTagPrefixKeepsColor(t *testing.T) {
	m := model{tags: []tag{{Name: "work", Color: "#ff00ff"}}, side: sideFirstProject + 1}
	for _, focus := range []bool{true, false} {
		m.focusSide = focus
		view := m.sidebarView()
		if !strings.Contains(view, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff00ff")).Render("  #work")) || strings.Contains(ansi.Strip(view), "● work") {
			t.Fatalf("tag prefix/color missing: %q", view)
		}
	}
}

func TestCompletionToastAndUndo(t *testing.T) {
	var statuses []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/task/task-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		statuses = append(statuses, body["status"])
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	original := task{UID: "task-1", Name: "Test", Status: "waiting"}
	m := model{api: newClient(config{BaseURL: server.URL}), page: pageAll, width: 100, height: 25, tasks: []task{original}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(model)
	if m.undoTask != nil {
		t.Fatal("toast must wait for API success")
	}
	if m.loading || !m.togglePending || !m.tasks[0].completed() || len(m.filteredTasks()) != 0 || strings.Contains(m.View(), "Loading…") {
		t.Fatal("completion must update immediately without a loader")
	}
	updated, _ = m.Update(cmd())
	m = updated.(model)
	if m.undoTask == nil || m.toast != "Well done 🎉" {
		t.Fatal("successful completion must show celebration")
	}
	generation := m.toastGeneration
	updated, _ = m.Update(celebrationTickMsg{generation})
	m = updated.(model)
	if m.toastFrame != 1 {
		t.Fatal("animation did not advance")
	}
	updated, _ = m.Update(loadedMsg{tasks: []task{}})
	m = updated.(model)
	if !strings.Contains(m.View(), "u  undo") || !strings.Contains(m.View(), "Well done 🎉") {
		t.Fatal("undo hint missing after refresh")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(model)
	updated, _ = m.Update(cmd())
	m = updated.(model)
	if len(statuses) != 2 || statuses[0] != "done" || statuses[1] != "waiting" || m.undoTask != nil {
		t.Fatalf("undo did not restore original status: %v", statuses)
	}
	updated, _ = m.Update(celebrationTickMsg{generation})
	if updated.(model).toastFrame != 0 {
		t.Fatal("stale timer changed newer toast")
	}
}

func TestOptimisticToggleRollback(t *testing.T) {
	for _, status := range []string{"waiting", "done"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "offline", http.StatusInternalServerError)
			}))
			defer server.Close()
			original := task{UID: "task-1", Name: "Test", Status: status, CompletedAt: "2026-10-20T10:00:00Z"}
			m := model{api: newClient(config{BaseURL: server.URL}), page: pageAll, showCompleted: original.completed(), tasks: []task{original}}
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
			m = updated.(model)
			if m.loading || m.tasks[0].completed() == original.completed() {
				t.Fatal("status must change before the API responds")
			}
			if _, duplicate := m.Update(tea.KeyMsg{Type: tea.KeySpace}); duplicate != nil {
				t.Fatal("must not send another toggle while one is pending")
			}
			updated, next := m.Update(cmd())
			m = updated.(model)
			if m.tasks[0].Status != original.Status || m.tasks[0].CompletedAt != original.CompletedAt || m.togglePending || m.err == "" || next != nil {
				t.Fatal("failure must restore the previous task and show an error")
			}
			if original.completed() {
				m.undoTask = &task{UID: original.UID, Status: "waiting"}
				updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
				m = updated.(model)
				if m.loading || m.tasks[0].completed() {
					t.Fatal("undo must update immediately without a loader")
				}
				updated, _ = m.Update(cmd())
				m = updated.(model)
				if !m.tasks[0].completed() || m.tasks[0].CompletedAt != original.CompletedAt || m.togglePending || m.err == "" {
					t.Fatal("failed undo must restore the completed task")
				}
			}
		})
	}
}

func TestCompletionToastFailureAndExpiry(t *testing.T) {
	original := task{UID: "task-1", Status: "in_progress"}
	m := model{loading: true}
	updated, _ := m.Update(taskToggleMsg{task: original, err: errors.New("offline")})
	m = updated.(model)
	if m.toast != "" || m.undoTask != nil || m.loading || m.err != "offline" {
		t.Fatal("failure must not celebrate")
	}
	updated, _ = m.Update(taskToggleMsg{task: original})
	m = updated.(model)
	m.toastFrame = 59
	updated, cmd := m.Update(celebrationTickMsg{m.toastGeneration})
	m = updated.(model)
	if m.toast != "" || m.undoTask != nil || cmd != nil {
		t.Fatal("toast and undo must expire together")
	}
	updated, _ = m.Update(taskToggleMsg{task: task{Status: "done"}})
	if updated.(model).undoTask != nil || updated.(model).toast != "Task reopened" {
		t.Fatal("reopening must not celebrate completion")
	}
}

func TestCalendarViewPersistsAcrossRestarts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := model{page: pageCalendar}
	for _, want := range []struct{ week, workingWeek bool }{{true, false}, {true, true}, {false, false}} {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		m = updated.(model)
		if cmd == nil {
			t.Fatal("changing calendar view should save state")
		}
		if msg := cmd().(stateSavedMsg); msg.err != nil {
			t.Fatal(msg.err)
		}
		state, err := loadState()
		if err != nil {
			t.Fatal(err)
		}
		restored := newModel(nil, state)
		if restored.week != want.week || restored.workingWeek != want.workingWeek {
			t.Fatalf("restored calendar view = (%v, %v), want (%v, %v)", restored.week, restored.workingWeek, want.week, want.workingWeek)
		}
	}
}

func TestLoadingAnimationLifecycle(t *testing.T) {
	for _, loadErr := range []error{nil, errors.New("offline")} {
		m := newModel(nil, appState{})
		m.width, m.height = 120, 30
		before := m.View()
		updated, cmd := m.Update(m.loader.Tick())
		m = updated.(model)
		if cmd == nil || m.View() == before || !strings.Contains(m.View(), "Tasks · Projects · Tags") {
			t.Fatal("loading spinner should animate and identify the data being loaded")
		}
		updated, _ = m.Update(loadedMsg{err: loadErr})
		m = updated.(model)
		updated, cmd = m.Update(m.loader.Tick())
		m = updated.(model)
		if m.loading || cmd != nil || strings.Contains(m.View(), "Tasks · Projects · Tags") {
			t.Fatal("animation should stop after loading succeeds or fails")
		}
		updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m = updated.(model)
		if !m.loading || cmd == nil {
			t.Fatal("refresh should restart loading")
		}
	}
}

func TestTaskSearchFiltersAndNavigation(t *testing.T) {
	m := model{page: pageAll, width: 100, height: 24, selectedProjectUID: "p", projects: []project{{ID: 1, UID: "p"}}, priorityFilter: "high", statusFilter: "in_progress", tagFilters: map[string]bool{"work": true}, tasks: []task{
		{ID: 1, Name: "First MATCH", ProjectID: 1, Priority: "high", Status: "in_progress", Tags: []tag{{Name: "Work"}}},
		{ID: 2, Name: "Second", Note: "match in description", ProjectID: 1, Priority: "high", Status: "in_progress", Tags: []tag{{Name: "Work"}}},
		{ID: 3, Name: "Match wrong priority", ProjectID: 1, Priority: "low", Status: "in_progress", Tags: []tag{{Name: "Work"}}},
		{ID: 4, Name: "Match wrong project", ProjectID: 2, Priority: "high", Status: "in_progress", Tags: []tag{{Name: "Work"}}},
		{ID: 5, Name: "Match wrong status", ProjectID: 1, Priority: "high", Tags: []tag{{Name: "Work"}}},
		{ID: 6, Name: "Match wrong tag", ProjectID: 1, Priority: "high", Status: "in_progress"},
	}}
	press := func(key tea.KeyMsg) { updated, _ := m.Update(key); m = updated.(model) }
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("MATCH")})
	if got := m.searchResults(); len(got) != 2 {
		t.Fatalf("filtered results: %#v", got)
	}
	press(tea.KeyMsg{Type: tea.KeyDown})
	if m.search.cursor != 1 {
		t.Fatal("down should select second result")
	}
	view := m.searchView()
	for _, line := range strings.Split(m.taskListItem(m.tasks[1], 60, true), "\n") {
		if !strings.Contains(view, line) {
			t.Fatal("results must reuse selected task rendering")
		}
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editor == nil || !m.editor.readOnly || m.editor.task.ID != 2 {
		t.Fatal("enter should open selected task in view mode")
	}
	if !strings.Contains(ansi.Strip(m.View()), "View task") {
		t.Fatal("view popup missing")
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.editor.readOnly {
		t.Fatal("e should enable editing")
	}
	m.editor.fields[0].SetValue("Unsaved change")
	press(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.editor.readOnly || m.editor.fields[0].Value() != "Second" {
		t.Fatal("escape should discard edits and return to view")
	}
	press(tea.KeyMsg{Type: tea.KeyEsc})
	if m.editor != nil || m.search == nil || m.search.cursor != 1 || m.search.input.Value() != "MATCH" {
		t.Fatal("escape should restore search and selection")
	}
	press(tea.KeyMsg{Type: tea.KeyUp})
	if m.search.cursor != 0 {
		t.Fatal("up should select first result")
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("missing")})
	if len(m.searchResults()) != 0 || m.search.cursor != 0 {
		t.Fatal("typing should update results and reset selection")
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editor != nil {
		t.Fatal("empty results should not open editor")
	}
	press(tea.KeyMsg{Type: tea.KeyEsc})
	if m.search != nil {
		t.Fatal("escape should close search")
	}
}

func TestTaskSearchRespectsDateFilterAndScrolls(t *testing.T) {
	m := model{page: pageCalendar, lastFilter: "upcoming", width: 80, height: 20, search: &taskSearch{input: input("", "Search")}}
	m.tasks = append(m.tasks, task{Name: "Unplanned"})
	for i := range 20 {
		m.tasks = append(m.tasks, task{ID: i + 1, Name: fmt.Sprintf("Task %02d", i), DueDate: time.Now().AddDate(0, 0, i+1).Format(time.RFC3339)})
	}
	if len(m.searchResults()) != 20 {
		t.Fatal("search should retain last selected date filter")
	}
	m.search.cursor = 19
	view := m.searchView()
	if !strings.Contains(ansi.Strip(view), "Task 19") || lipgloss.Height(view) > m.height {
		t.Fatal("selected result should remain visible within terminal")
	}
	m.search.cursor = 0
	updated, _ := m.updateSearch(tea.KeyMsg{Type: tea.KeyUp})
	if updated.(model).search.cursor != 0 {
		t.Fatal("up should stay within results")
	}
}

func TestTaskSearchSaveReturnsToUpdatedView(t *testing.T) {
	original := task{ID: 7, UID: "task", Name: "Original"}
	m := model{page: pageAll, search: &taskSearch{input: input("", "Search")}}
	m.openTaskEditor(original)
	updated, _ := m.Update(actionMsg{})
	m = updated.(model)
	if m.editor == nil || !m.editor.readOnly || m.search == nil {
		t.Fatal("save should return to task view within search")
	}
	saved := original
	saved.Name = "Saved title"
	updated, _ = m.Update(loadedMsg{tasks: []task{saved}})
	m = updated.(model)
	if m.editor.task.Name != "Saved title" || !m.editor.readOnly {
		t.Fatal("task view should show refreshed saved data")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if got := m.searchResults(); len(got) != 1 || got[0].Name != "Saved title" {
		t.Fatal("results should use refreshed data")
	}
}

func TestTaskSearchAllTasksCheckbox(t *testing.T) {
	m := model{page: pageUnplanned, width: 80, height: 20, selectedProjectUID: "p", projects: []project{{ID: 1, UID: "p"}}, priorityFilter: "high", statusFilter: "in_progress", tagFilters: map[string]bool{"work": true}, search: &taskSearch{input: input("match", "Search")}, tasks: []task{
		{ID: 1, Name: "Match filtered", ProjectID: 1, Priority: "high", Status: "in_progress", Tags: []tag{{Name: "work"}}},
		{ID: 2, Name: "Match completed", Status: "done", DueDate: "2027-01-01"},
		{ID: 3, Name: "Other", Status: "done"},
	}}
	m.search.input.Focus()
	press := func(key tea.KeyMsg) { updated, _ := m.Update(key); m = updated.(model) }
	if len(m.searchResults()) != 1 {
		t.Fatal("search should initially use filters")
	}
	press(tea.KeyMsg{Type: tea.KeyTab})
	if !m.search.focusAll || m.search.input.Focused() {
		t.Fatal("tab should focus checkbox")
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !m.search.allTasks || len(m.searchResults()) != 2 || m.search.input.Value() != "match" {
		t.Fatal("checkbox should bypass every filter while keeping the query")
	}
	if !strings.Contains(ansi.Strip(m.searchView()), "[x] all tasks") || lipgloss.Height(m.searchView()) > m.height {
		t.Fatal("checkbox should be visible within terminal")
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.search.allTasks || len(m.searchResults()) != 1 || m.editor != nil {
		t.Fatal("enter on checkbox should restore filters")
	}
	press(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.search.focusAll || !m.search.input.Focused() {
		t.Fatal("shift-tab should return to search input")
	}
	if m.selectedProjectUID != "p" || m.priorityFilter != "high" || m.statusFilter != "in_progress" || !m.tagFilters["work"] || m.page != pageUnplanned {
		t.Fatal("all tasks must preserve selected filters")
	}
}

func TestTaskSearchPopupPositionStaysFixed(t *testing.T) {
	for _, height := range []int{20, 24, 40} {
		m := model{page: pageAll, width: 100, height: height, search: &taskSearch{input: input("", "Search")}}
		for i := range 30 {
			m.tasks = append(m.tasks, task{Name: fmt.Sprintf("Task %02d", i)})
		}
		var top, bottom, fieldRow, hintRow int
		for i, query := range []string{"", "Task 01", "missing"} {
			m.search.input.SetValue(query)
			view := ansi.Strip(m.searchView())
			if lipgloss.Height(view) != height {
				t.Fatalf("height %d: popup exceeds available space", height)
			}
			var gotTop, gotBottom, gotField, gotHint int
			for row, line := range strings.Split(view, "\n") {
				if strings.Contains(line, "╭") {
					gotTop = row
				}
				if strings.Contains(line, "╰") {
					gotBottom = row
				}
				if strings.Contains(line, "[ ] all tasks") {
					gotField = row
				}
				if strings.Contains(line, "tab focus") {
					gotHint = row
				}
			}
			if i == 0 {
				top, bottom, fieldRow, hintRow = gotTop, gotBottom, gotField, gotHint
			} else if top != gotTop || bottom != gotBottom || fieldRow != gotField || hintRow != gotHint {
				t.Fatalf("height %d: layout moved for query %q", height, query)
			}
			if bottom-top+1 != height-4 {
				t.Fatalf("height %d: popup should use available height", height)
			}
		}
	}
}

func TestTaskSearchCheckboxAndResultsHighlightAreExclusive(t *testing.T) {
	for _, arrow := range []tea.KeyType{tea.KeyUp, tea.KeyDown} {
		m := model{page: pageAll, width: 100, height: 24, search: &taskSearch{input: input("", "Search"), cursor: 1}, tasks: []task{{Name: "First"}, {Name: "Second"}, {Name: "Third"}}}
		press := func(key tea.KeyMsg) { updated, _ := m.Update(key); m = updated.(model) }
		hasItem := func(active bool) bool {
			for _, line := range strings.Split(m.taskListItem(m.tasks[1], 60, active), "\n") {
				if !strings.Contains(m.searchView(), line) {
					return false
				}
			}
			return true
		}
		press(tea.KeyMsg{Type: tea.KeyTab})
		if !m.search.focusAll || !hasItem(false) {
			t.Fatal("checkbox focus should remove task highlight")
		}
		if !strings.Contains(m.searchView(), selected.Render("[ ] all tasks")) {
			t.Fatal("checkbox should be highlighted")
		}
		press(tea.KeyMsg{Type: arrow})
		if m.search.focusAll || m.search.cursor != 1 || !hasItem(true) {
			t.Fatal("arrow should restore highlight to current result")
		}
		if !strings.Contains(m.searchView(), dim.Render("[ ] all tasks")) {
			t.Fatal("checkbox highlight should disappear")
		}
		press(tea.KeyMsg{Type: tea.KeyEnter})
		if m.editor == nil || m.editor.task.Name != "Second" {
			t.Fatal("enter should open result after focus transfer")
		}
	}
}

func TestAllTasksDateSections(t *testing.T) {
	now := time.Now()
	m := model{page: pageAll, height: 40, sortBy: "name", sortDesc: true, tasks: []task{
		{Name: "A future", DueDate: now.AddDate(0, 0, 1).Format("2006-01-02")},
		{Name: "B today", DueDate: now.Format("2006-01-02")},
		{Name: "C late", DueDate: now.AddDate(0, 0, -1).Format("2006-01-02")},
		{Name: "D unplanned"},
		{Name: "Z late", DueDate: now.AddDate(0, 0, -2).Format("2006-01-02")},
	}}
	view := ansi.Strip(m.contentView(60))
	previous := -1
	for _, text := range []string{"\n\n                            Late                            \n", "Z late", "C late", "\n\n                           Today                            \n", "B today", "\n\n                          Upcoming                          \n", "A future", "\n\n                         Unplanned                          \n", "D unplanned"} {
		index := strings.Index(view, text)
		if index <= previous {
			t.Fatalf("missing or misplaced %q in %q", text, view)
		}
		previous = index
	}
	m.cursor = 2
	if selected, ok := m.selectedTask(); !ok || selected.Name != "B today" {
		t.Fatalf("selection does not follow grouped order: %#v", selected)
	}
	m.tasks = m.tasks[:1]
	view = ansi.Strip(m.contentView(60))
	if strings.Contains(view, "\n\n                            Late                            \n") || strings.Contains(view, "\n\n                           Today                            \n") || !strings.Contains(view, "\n\n                          Upcoming                          \n") {
		t.Fatalf("empty sections should be omitted: %q", view)
	}
	m.page = pageUpcoming
	if view = ansi.Strip(m.contentView(60)); strings.Contains(view, "\n\n                          Upcoming                          \n") {
		t.Fatalf("section separator leaked outside All: %q", view)
	}
}

func TestAllTasksSortByDateThenPriority(t *testing.T) {
	m := model{page: pageAll, sortBy: "name", sortDesc: true, tasks: []task{
		{Name: "later high", DueDate: "2030-05-02", Priority: "high"},
		{Name: "early low", DueDate: "2030-05-01", Priority: "low"},
		{Name: "unplanned low", Priority: "low"},
		{Name: "early high", DueDate: "2030-05-01", Priority: "high"},
		{Name: "early medium", DueDate: "2030-05-01", Priority: "medium"},
		{Name: "unplanned high", Priority: "high"},
	}}
	want := []string{"early high", "early medium", "early low", "later high", "unplanned high", "unplanned low"}
	for i, task := range m.filteredTasks() {
		if task.Name != want[i] {
			t.Fatalf("task %d = %q, want %q", i, task.Name, want[i])
		}
	}
}

func TestCalendarSelectionVisibleWithoutColor(t *testing.T) {
	day := time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local)
	for _, mode := range []struct{ week, working bool }{{}, {week: true}, {week: true, working: true}} {
		m := model{page: pageCalendar, width: 160, height: 30, calDate: day, week: mode.week, workingWeek: mode.working}
		view := ansi.Strip(m.calendarView(130))
		label := "▸23"
		if mode.week {
			label = "▸ Friday 23"
		}
		if !strings.Contains(view, label) {
			t.Fatalf("mode=%+v: selected date has no visible cursor", mode)
		}
	}
}

func TestCalendarTaskCursorScrollsAndOpensVisibleTask(t *testing.T) {
	day := time.Now()
	m := model{page: pageCalendar, width: 100, height: 30, calDate: day, calendarTasks: true}
	for i := 0; i < 12; i++ {
		m.tasks = append(m.tasks, task{UID: fmt.Sprint(i), Name: fmt.Sprintf("task %02d", i), DueDate: day.Format(time.RFC3339)})
	}
	for i := 0; i < 11; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	// Refreshing data must preserve the selected task instead of resetting to zero.
	updated, _ := m.Update(loadedMsg{tasks: m.tasks})
	m = updated.(model)
	view := ansi.Strip(m.calendarView(75))
	if m.cursor != 11 || !strings.Contains(view, "▸ task 11") {
		t.Fatalf("selected task is not visible: cursor=%d\n%s", m.cursor, view)
	}
	if lipgloss.Height(m.View()) > m.height {
		t.Fatal("task popup exceeds terminal height")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if view := ansi.Strip(m.calendarView(75)); !strings.Contains(view, "▸ task 10") || strings.Contains(view, "▸ task 11") {
		t.Fatalf("visible cursor did not move up:\n%s", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.editor == nil || m.editor.task.UID != "10" {
		t.Fatal("enter did not open the visibly selected task")
	}
}

func TestCalendarCellOverflowAndPopupHighlight(t *testing.T) {
	day := time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local)
	for _, mode := range []struct {
		working bool
		height  int
	}{{false, 38}, {true, 30}} {
		m := model{page: pageCalendar, width: 160, height: mode.height, week: true, workingWeek: mode.working, calDate: day}
		for i := 0; i < 3; i++ {
			m.tasks = append(m.tasks, task{Name: fmt.Sprintf("task %d", i), DueDate: day.Format(time.RFC3339)})
		}
		view := ansi.Strip(m.weekView(130))
		if !strings.Contains(view, "+ 2") || !strings.Contains(view, "task 0") || strings.Contains(view, "task 1") {
			t.Fatalf("overflow must show one complete task and count the other two:\n%s", view)
		}
		m.calendarTasks = true
		view = ansi.Strip(m.weekView(130))
		if strings.Count(view, "▸") != 2 {
			t.Fatalf("only the day heading and popup task should have a cursor:\n%s", view)
		}
		if got := m.calendarSelection(130); got != dim.Render(day.Format("Monday, 02 January 2006")) {
			t.Fatalf("top date should use the normal dim style: %q", got)
		}
	}
}

func TestMonthOverflowCountFitsBesideLongTaskName(t *testing.T) {
	day := time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local)
	m := model{page: pageCalendar, width: 160, height: 30, calDate: day}
	for i := 0; i < 3; i++ {
		m.tasks = append(m.tasks, task{Name: strings.Repeat("long task name ", 10), DueDate: day.Format(time.RFC3339)})
	}
	if view := ansi.Strip(m.calendarView(130)); !strings.Contains(view, "+ 2") {
		t.Fatalf("month cell lost its overflow count:\n%s", view)
	}
}

func TestCalendarHorizontalBoundaries(t *testing.T) {
	monday := time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local)
	for _, mode := range []struct {
		name          string
		week, working bool
		days, columns int
	}{
		{"month", false, false, 7, 7},
		{"week", true, false, 7, 2},
		{"working week", true, true, 5, 2},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for day := 0; day < mode.days; day++ {
				for _, key := range []tea.KeyType{tea.KeyLeft, tea.KeyRight} {
					date := monday.AddDate(0, 0, day)
					m := model{page: pageCalendar, calDate: date, week: mode.week, workingWeek: mode.working}
					wantDate, wantSidebar := date, false
					if key == tea.KeyLeft {
						if day%mode.columns == 0 {
							wantSidebar = true
						} else {
							wantDate = date.AddDate(0, 0, -1)
						}
					} else if day%mode.columns != mode.columns-1 && day < mode.days-1 {
						wantDate = date.AddDate(0, 0, 1)
					}
					updated, _ := m.Update(tea.KeyMsg{Type: key})
					got := updated.(model)
					if got.focusSide != wantSidebar || !sameDay(got.calDate, wantDate) {
						t.Fatalf("day %d key %v: sidebar=%v date=%v, want sidebar=%v date=%v", day, key, got.focusSide, got.calDate, wantSidebar, wantDate)
					}
				}
			}
		})
	}
}

func TestCalendarSelectionFollowsFocus(t *testing.T) {
	for _, mode := range []struct{ week, working bool }{{false, false}, {true, false}, {true, true}} {
		m := model{page: pageCalendar, width: 160, height: 40, week: mode.week, workingWeek: mode.working, calDate: time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local)}
		if !strings.Contains(m.calendarView(130), "▸") {
			t.Fatal("focused calendar should highlight the selected day")
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		m = updated.(model)
		if !m.focusSide || strings.Contains(m.calendarView(130), "▸") {
			t.Fatal("calendar should clear its highlight when focus returns to the sidebar")
		}
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(model)
		if m.focusSide || !strings.Contains(m.calendarView(130), "▸") || m.calDate.Day() != 19 {
			t.Fatal("returning to calendar should restore the same selected day")
		}
	}
}

func TestCompletedTaskMetadataIsStruck(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	item := task{Name: "Finished", Status: "done", Priority: "high", DueDate: "2026-10-23", Project: &project{Name: "Work", Color: "#ff00ff"}, Tags: []tag{{Name: "urgent", Color: "#00ff00"}}}
	for _, active := range []bool{false, true} {
		line := strings.Split((model{}).taskListItem(item, 80, active), "\n")[1]
		if active {
			want := selected.Width(80).Faint(true).Strikethrough(true).Render(ansi.Strip(line))
			if line != want {
				t.Fatalf("selected metadata should be struck: %q", line)
			}
		} else {
			for _, want := range []string{
				lipgloss.NewStyle().Foreground(lipgloss.Color("#ff00ff")).Strikethrough(true).Render("● Work"),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Strikethrough(true).Render("#urgent"),
				dim.Strikethrough(true).Render("◷ Oct 23"),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444")).Bold(true).Strikethrough(true).Render("H"),
			} {
				if !strings.Contains(line, want) {
					t.Fatalf("missing struck metadata %q in %q", want, line)
				}
			}
		}
	}
}

func TestTaskListDateRightBeforePriority(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, name := range []string{"Work", strings.Repeat("Long project", 10)} {
			item := task{Name: "Task", DueDate: "2030-10-23", Priority: "high", Project: &project{Name: name}, Tags: []tag{{Name: "urgent"}}}
			line := ansi.Strip(strings.Split((model{}).taskListItem(item, 60, active), "\n")[1])
			if !strings.HasSuffix(line, "◷ Oct 23  H") || lipgloss.Width(line) != 60 {
				t.Fatalf("date and priority should align at the right edge: %q", line)
			}
		}
	}
}
