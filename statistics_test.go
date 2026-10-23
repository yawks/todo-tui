package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestStatisticsPeriods(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for i, want := range []string{"2026-10-19/2026-10-24", "2026-10-12/2026-10-19", "2026-09-28/2026-10-24", "2026-08-03/2026-10-24", "2026-05-01/2026-10-24", "2025-11-01/2026-10-24"} {
		start, end := statisticsRange(now, i)
		if got := start.Format("2006-01-02") + "/" + end.Format("2006-01-02"); got != want {
			t.Errorf("period %d: %s", i, got)
		}
	}
}

func TestPeriodStatistics(t *testing.T) {
	loc := time.FixedZone("local", 2*60*60)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, loc)
	m := model{tasks: []task{
		{Name: "late", Status: "done", DueDate: "2026-10-19", CreatedAt: "2026-10-19T00:00:00+02:00", CompletedAt: "2026-10-19T23:30:00Z"},
		{Name: "on time", Status: "done", DueDate: "2026-10-20", CompletedAt: "2026-10-20T10:00:00+02:00"},
		{Name: "slipped", Status: "planned", DueDate: "2026-10-21"},
		{Name: "overdue", Status: "not_started", DueDate: "2026-09-01"},
		{Name: "cancelled", Status: "cancelled", DueDate: "2026-10-21"},
		{Name: "undated", Status: "done", UpdatedAt: "2026-10-20T10:00:00Z"},
	}}
	s := m.periodStatistics(now)
	if s.completed != 2 || s.created != 1 || s.overdue != 2 || len(s.late) != 1 || len(s.slipped) != 1 || s.undated != 1 || s.daily["2026-10-20"] != 2 {
		t.Fatalf("unexpected stats: %+v", s)
	}
	m.statsPeriod = 1
	m.tasks = append(m.tasks, task{Name: "completed later", Status: "done", DueDate: "2026-10-15", CompletedAt: "2026-10-20T10:00:00Z"})
	s = m.periodStatistics(now)
	if len(s.slipped) != 1 || s.completed != 0 {
		t.Fatalf("historical stats: %+v", s)
	}
}

func TestStatisticsNavigationAndLayout(t *testing.T) {
	m := newModel(nil, appState{Filter: "statistics"})
	m.loading = false
	m.width, m.height = 80, 20
	if m.page != pageStatistics || m.side != sideStatistics {
		t.Fatal("statistics not restored")
	}
	m.focusSide = false
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(model)
	if m.statsPeriod != 0 || m.statsPeriodCursor != 1 {
		t.Fatal("period applied before enter")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.statsPeriod != 1 {
		t.Fatal("period not applied")
	}
	if _, ok := m.selectedTask(); ok {
		t.Fatal("statistics selects a task")
	}
	for _, size := range [][2]int{{80, 20}, {120, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("view exceeds width %d: %d", m.width, lipgloss.Width(line))
			}
		}
		if lipgloss.Height(m.View()) > m.height {
			t.Fatalf("view exceeds height %d: %d", m.height, lipgloss.Height(m.View()))
		}
	}
}

func TestStatisticsTaskBrowsing(t *testing.T) {
	now := time.Now()
	m := model{page: pageStatistics, width: 120, height: 30}
	for i := 0; i < 40; i++ {
		m.tasks = append(m.tasks, task{ID: i + 1, Name: fmt.Sprintf("Task %02d", i), Status: "done", DueDate: now.AddDate(0, 0, -1).Format("2006-01-02"), CompletedAt: now.Format(time.RFC3339)})
	}
	before := strings.Split(m.statisticsView(90), "\n")
	m.updateStatisticsKey("down")
	if !m.statsTasksFocus || m.statsCursor != 0 {
		t.Fatal("down must focus first task")
	}
	m.updateStatisticsKey("end")
	if m.statsCursor != 39 {
		t.Fatal("end must select last task")
	}
	after := strings.Split(m.statisticsView(90), "\n")
	for i, line := range before {
		if strings.Contains(line, "TASKS") {
			break
		}
		if after[i] != line {
			t.Fatal("summary moved while scrolling tasks")
		}
	}
	m.updateStatisticsKey("enter")
	if m.editor == nil || !m.editor.readOnly || m.editor.task.ID != 40 {
		t.Fatal("task not opened read only")
	}
	updated, _ := m.updateEditor(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = updated.(model)
	if m.editor.readOnly {
		t.Fatal("e must enable editing from statistics")
	}
	updated, _ = m.updateEditor(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.editor != nil || m.statsCursor != 39 {
		t.Fatal("popup did not preserve selection")
	}
	m.updateStatisticsKey("home")
	m.updateStatisticsKey("up")
	if m.statsTasksFocus {
		t.Fatal("up from first task must focus periods")
	}
}

func TestStatisticsCompletionGrouping(t *testing.T) {
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		period  int
		title   string
		buckets int
		first   string
		last    string
	}{
		{0, "Completed per day", 6, "Mon 26 Jan", "Sat 31 Jan"},
		{1, "Completed per day", 7, "Mon 19 Jan", "Sun 25 Jan"},
		{2, "Completed per week", 4, "05 Jan 2026", "26 Jan 2026"},
		{3, "Completed per week", 12, "10 Nov 2025", "26 Jan 2026"},
		{4, "Completed per month", 6, "Aug 2025", "Jan 2026"},
		{5, "Completed per month", 12, "Feb 2025", "Jan 2026"},
	} {
		t.Run(tc.title+fmt.Sprint(tc.period), func(t *testing.T) {
			start, end := statisticsRange(now, tc.period)
			s := periodStatistics{start: start, end: end, daily: map[string]int{}}
			s.daily[start.Format("2006-01-02")] = 2
			s.daily[start.AddDate(0, 0, 1).Format("2006-01-02")] = 3
			s.daily[end.AddDate(0, 0, -1).Format("2006-01-02")] = 7
			title, labels, counts := s.completionSeries(tc.period)
			if title != tc.title || len(labels) != tc.buckets || labels[0] != tc.first || labels[len(labels)-1] != tc.last {
				t.Fatalf("unexpected series: %s %v", title, labels)
			}
			total := 0
			for _, n := range counts {
				total += n
			}
			if total != 12 {
				t.Fatalf("grouping lost completions: %v", counts)
			}
			if tc.period >= 2 && counts[0] != 5 {
				t.Fatalf("first bucket: %v", counts)
			}
			if tc.period >= 2 && counts[1] != 0 {
				t.Fatalf("empty bucket omitted: %v", counts)
			}
		})
	}
}

func TestStatisticsMonthlyPeriodNavigation(t *testing.T) {
	m := model{page: pageStatistics, width: 80, height: 20}
	for i := 0; i < 10; i++ {
		m.updateStatisticsKey("right")
	}
	if m.statsPeriodCursor != 5 || m.statsPeriod != 0 {
		t.Fatal("last year must be selectable without applying")
	}
	if !strings.Contains(m.statisticsView(54), "Last year") {
		t.Fatal("selected tab must remain visible")
	}
	m.updateStatisticsKey("enter")
	if m.statsPeriod != 5 {
		t.Fatal("last year not applied")
	}
	for _, size := range [][2]int{{80, 20}, {120, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for period := range statisticsPeriods {
			m.statsPeriod, m.statsPeriodCursor = period, period
			view := m.View()
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("period %d exceeds width %d", period, m.width)
				}
			}
			if lipgloss.Height(view) > m.height {
				t.Fatalf("period %d exceeds height %d", period, m.height)
			}
		}
	}
}

func TestStatisticsCompletedTaskList(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	m := model{tasks: []task{
		{Name: "undated", Status: "done", CompletedAt: "2026-10-20T12:00:00Z"},
		{Name: "older", Status: "done", DueDate: "2026-10-19", CompletedAt: "2026-10-20T12:00:00Z"},
		{Name: "newer", Status: "done", DueDate: "2026-10-23T00:00:00+02:00", CompletedAt: "2026-10-22T12:00:00Z"},
		{Name: "open", Status: "planned", DueDate: "2026-10-22"},
		{Name: "outside", Status: "done", CompletedAt: "2026-10-10T12:00:00Z"},
	}}
	s := m.periodStatistics(now)
	rows := s.taskRows()
	if len(rows) != s.completed || len(rows) != 3 {
		t.Fatalf("expected all period completions, got %v", rows)
	}
	for i, want := range []string{"newer", "older", "undated"} {
		if rows[i].Name != want {
			t.Fatalf("row %d: %s", i, rows[i].Name)
		}
	}
}

func TestStatisticsTaskLineColors(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := model{projects: []project{{ID: 1, Name: "Project", Color: "#ff0000"}}}
	task := task{Name: "Task", ProjectID: 1, DueDate: "2026-10-23", Tags: []tag{{Name: "Tag", Color: "#00ff00"}}}
	for _, active := range []bool{false, true} {
		line := m.statisticsTaskLine(task, 80, active)
		plain := strings.TrimSpace(ansi.Strip(line))
		if plain != "23 Oct 2026 · Task · Project  #Tag" {
			t.Fatalf("unexpected line: %q", plain)
		}
		if active {
			if line != selected.Width(80).Render(plain) {
				t.Fatalf("selected row must use white text throughout: %q", line)
			}
		} else if !strings.Contains(line, "38;2;255;0;0") || !strings.Contains(line, "38;2;0;255;0") {
			t.Fatalf("project/tag colors lost: %q", line)
		}
		if lipgloss.Width(line) != 80 {
			t.Fatalf("unexpected width %d", lipgloss.Width(line))
		}
	}
	task.DueDate = ""
	if plain := ansi.Strip(m.statisticsTaskLine(task, 54, false)); !strings.HasPrefix(plain, "— · Task") {
		t.Fatalf("undated row: %s", plain)
	}
	task.Name = strings.Repeat("long title ", 30)
	line := ansi.Strip(m.statisticsTaskLine(task, 54, true))
	if !strings.Contains(line, "Project  #Tag") || lipgloss.Width(line) != 54 {
		t.Fatalf("metadata truncated: %q", line)
	}
}
