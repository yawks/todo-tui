package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var statisticsPeriods = []string{"This week", "Last week", "Last 4 weeks", "Last 12 weeks", "Last 6 months", "Last year"}

type periodStatistics struct {
	start, end                           time.Time
	completed, created, overdue, undated int
	late, slipped                        []task
	completedTasks                       []task
	daily                                map[string]int
	projects                             map[string]int
}

func statisticsRange(now time.Time, period int) (time.Time, time.Time) {
	today := dayStart(now)
	monday := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	switch period {
	case 1:
		return monday.AddDate(0, 0, -7), monday
	case 2:
		return monday.AddDate(0, 0, -21), today.AddDate(0, 0, 1)
	case 3:
		return monday.AddDate(0, 0, -77), today.AddDate(0, 0, 1)
	case 4, 5:
		months := 6
		if period == 5 {
			months = 12
		}
		month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		return month.AddDate(0, 1-months, 0), today.AddDate(0, 0, 1)
	default:
		return monday, today.AddDate(0, 0, 1)
	}
}

func (m model) periodStatistics(now time.Time) periodStatistics {
	start, end := statisticsRange(now, m.statsPeriod)
	s := periodStatistics{start: start, end: end, daily: map[string]int{}, projects: map[string]int{}}
	today := dayStart(now)
	within := func(d time.Time) bool { return !d.Before(start) && d.Before(end) && !d.After(now) }
	for _, t := range m.tasks {
		if !m.matchesSelectedProject(t) {
			continue
		}
		if d, ok := t.created(); ok && within(d) {
			s.created++
		}
		due, hasDue := t.due()
		// Due dates describe a calendar day, regardless of an optional timestamp offset.
		due = time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, now.Location())
		completed, err := time.Parse(time.RFC3339, t.CompletedAt)
		hasCompletion := err == nil && t.completed()
		if t.completed() && !hasCompletion {
			s.undated++
		}
		if hasCompletion && within(completed) {
			s.completed++
			s.completedTasks = append(s.completedTasks, t)
			s.daily[completed.In(now.Location()).Format("2006-01-02")]++
			name := "No project"
			if p := m.taskProject(t); p != nil {
				name = p.Name
			}
			s.projects[name]++
			if hasDue && dayStart(completed.In(now.Location())).After(due) {
				s.late = append(s.late, t)
			}
		}
		cancelled := statusName(t.Status) == "cancelled"
		if hasDue && !t.completed() && !cancelled && due.Before(today) {
			s.overdue++
		}
		if hasDue && !cancelled && !due.Before(start) && due.Before(end) &&
			((!t.completed()) || (hasCompletion && !completed.Before(end))) {
			s.slipped = append(s.slipped, t)
		}
	}
	sortTasks := func(ts []task) {
		sort.SliceStable(ts, func(i, j int) bool {
			if ts[i].DueDate == ts[j].DueDate {
				return ts[i].Name < ts[j].Name
			}
			return ts[i].DueDate < ts[j].DueDate
		})
	}
	sortTasks(s.late)
	sortTasks(s.slipped)
	return s
}

// Completion buckets include empty days, weeks or months within the selected period.
func (s periodStatistics) completionSeries(period int) (string, []string, []int) {
	title := "Completed per day"
	if period == 2 || period == 3 {
		title = "Completed per week"
	}
	if period >= 4 {
		title = "Completed per month"
	}
	var labels []string
	var counts []int
	var previous time.Time
	for d := s.start; d.Before(s.end); d = d.AddDate(0, 0, 1) {
		bucket := d
		label := d.Format("Mon 02 Jan")
		if period == 2 || period == 3 {
			bucket = d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
			label = bucket.Format("02 Jan 2006")
		} else if period >= 4 {
			bucket = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
			label = bucket.Format("Jan 2006")
		}
		if !bucket.Equal(previous) {
			labels = append(labels, label)
			counts = append(counts, 0)
			previous = bucket
		}
		counts[len(counts)-1] += s.daily[d.Format("2006-01-02")]
	}
	return title, labels, counts
}

func (s periodStatistics) taskRows() []task {
	rows := append([]task(nil), s.completedTasks...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, hasA := rows[i].due()
		b, hasB := rows[j].due()
		if hasA != hasB {
			return hasA
		}
		if hasA && a.Format("2006-01-02") != b.Format("2006-01-02") {
			return a.Format("2006-01-02") > b.Format("2006-01-02")
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func statisticsTaskGroup(t task, period int) string {
	d, ok := t.due()
	if !ok {
		return "No due date"
	}
	switch period {
	case 0, 1:
		return d.Format("Mon 02 Jan 2006")
	case 2:
		monday := d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
		return "Week of " + monday.Format("02 Jan 2006")
	default:
		return d.Format("Jan 2006")
	}
}

func (m model) statisticsTaskLine(t task, width int, active bool) string {
	base := lipgloss.NewStyle()
	if active {
		base = selected
	}
	colored := func(text, color string) string {
		style := base
		if color != "" && !active {
			style = style.Foreground(lipgloss.Color(color))
		}
		return style.Render(text)
	}
	date := "—"
	if d, ok := t.due(); ok {
		date = d.Format("02 Jan 2006")
	}
	projectName := colored("No project", "")
	if p := m.taskProject(t); p != nil {
		projectName = colored(p.Name, p.Color)
	}
	suffix := base.Render(" · ") + projectName
	for _, tag := range t.Tags {
		suffix += base.Render("  ") + colored("#"+tag.Name, tag.Color)
	}
	prefix := base.Render(date + " · ")
	titleWidth := max(1, width-lipgloss.Width(prefix)-lipgloss.Width(suffix))
	line := prefix + base.Render(ansi.Truncate(t.Name, titleWidth, "…")) + suffix
	if active {
		line = ansi.Strip(line)
	}
	return base.Width(width).Render(ansi.Truncate(line, width, "…"))
}

func (m *model) updateStatisticsKey(key string) bool {
	rows := m.periodStatistics(time.Now()).taskRows()
	switch key {
	case "left":
		if m.statsTasksFocus || m.statsPeriodCursor == 0 {
			m.focusSide = true
		} else {
			m.statsPeriodCursor--
		}
	case "[", "]", "right":
		if m.statsTasksFocus {
			return true
		}
		step := 1
		if key == "[" {
			step = -1
		}
		m.statsPeriodCursor = max(0, min(len(statisticsPeriods)-1, m.statsPeriodCursor+step))
	case "enter":
		if !m.statsTasksFocus {
			m.statsPeriod = m.statsPeriodCursor
			m.statsCursor = 0
		} else if len(rows) > 0 {
			m.statsCursor = min(m.statsCursor, len(rows)-1)
			m.openTaskEditor(rows[m.statsCursor])
			m.editor.readOnly = true
			m.detailScroll = 0
		}
	case "down", "j":
		if !m.statsTasksFocus {
			m.statsTasksFocus = len(rows) > 0
			m.statsCursor = 0
		} else {
			m.statsCursor = min(max(0, len(rows)-1), m.statsCursor+1)
		}
	case "up", "k":
		if m.statsTasksFocus && m.statsCursor > 0 {
			m.statsCursor--
		} else {
			m.statsTasksFocus = false
		}
	case "pgup", "pgdown", "home", "end":
		if !m.statsTasksFocus {
			return true
		}
		switch key {
		case "home":
			m.statsCursor = 0
		case "end":
			m.statsCursor = max(0, len(rows)-1)
		case "pgup":
			m.statsCursor = max(0, m.statsCursor-5)
		case "pgdown":
			m.statsCursor = min(max(0, len(rows)-1), m.statsCursor+5)
		}
	case "esc":
		if m.statsTasksFocus {
			m.statsTasksFocus = false
		} else {
			m.focusSide = true
		}
	default:
		return false
	}
	return true
}

func (m model) statisticsView(width int) string {
	s := m.periodStatistics(time.Now())
	var tabs []string
	for i, label := range statisticsPeriods {
		text := label
		if i == m.statsPeriod {
			text = "✓ " + text
		}
		if i == m.statsPeriodCursor && !m.statsTasksFocus && !m.focusSide {
			text = selected.Render(text)
		} else {
			text = dim.Render(text)
		}
		tabs = append(tabs, text)
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Statistics"))
	if p := m.selectedProject(); p != nil {
		b.WriteString(" · " + p.Name)
	}
	tabStart := 0
	activeTab := m.statsPeriodCursor
	if m.statsTasksFocus || m.focusSide {
		activeTab = m.statsPeriod
	}
	for tabStart < activeTab && lipgloss.Width(strings.Join(tabs[tabStart:activeTab+1], "  ")) > width-4 {
		tabStart++
	}
	tabLine := strings.Join(tabs[tabStart:], "  ")
	if tabStart > 0 {
		tabLine = "‹ " + tabLine
	}
	b.WriteString("\n" + ansi.Truncate(tabLine, width, " ›") + "\n")
	b.WriteString(dim.Render(s.start.Format("02 Jan 2006")+" – "+s.end.AddDate(0, 0, -1).Format("02 Jan 2006")) + "\n")
	b.WriteString(dim.Render("←/→ choose · ← first: sidebar · enter apply/open · ↓ tasks · esc back") + "\n")
	cards := []struct {
		name  string
		count int
	}{{"Completed", s.completed}, {"Created", s.created}, {"Completed late", len(s.late)}, {"Slipped", len(s.slipped)}, {"Overdue now", s.overdue}}
	columns := min(5, max(1, width/19))
	if m.height < 30 {
		var totals []string
		for _, c := range cards {
			totals = append(totals, fmt.Sprintf("%s %d", c.name, c.count))
		}
		line := ""
		for _, total := range totals {
			if line != "" && lipgloss.Width(line+" · "+total) > width {
				b.WriteString(line + "\n")
				line = ""
			}
			if line != "" {
				line += " · "
			}
			line += total
		}
		b.WriteString(line + "\n")
	} else {
		for i := 0; i < len(cards); i += columns {
			var row []string
			for _, c := range cards[i:min(len(cards), i+columns)] {
				row = append(row, dashboardPanel(c.name, fmt.Sprint(c.count), width/columns))
			}
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, row...) + "\n")
		}
	}
	chartTitle, daily, counts := s.completionSeries(m.statsPeriod)
	chart := func(labels []string, values []int, w int) string {
		largest := 0
		for _, n := range values {
			largest = max(largest, n)
		}
		var rows []string
		labelWidth := min(18, max(8, w/3))
		for i, label := range labels {
			rows = append(rows, fmt.Sprintf("%-*s %s %d", labelWidth, truncate(label, labelWidth), countBar(values[i], largest, max(1, w-labelWidth-9), "#3b82f6"), values[i]))
		}
		if len(rows) == 0 {
			return dim.Render("Nothing here")
		}
		limit := max(1, (m.height-19)/2)
		if len(rows) > limit {
			rows = append(rows[:limit], dim.Render(fmt.Sprintf("… %d more", len(rows)-limit)))
		}
		return strings.Join(rows, "\n")
	}
	var names []string
	for name := range s.projects {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if s.projects[names[i]] == s.projects[names[j]] {
			return names[i] < names[j]
		}
		return s.projects[names[i]] > s.projects[names[j]]
	})
	var values []int
	for _, name := range names {
		values = append(values, s.projects[name])
	}
	if width >= 90 || m.height < 30 {
		w := width / 2
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, dashboardPanel(chartTitle, chart(daily, counts, w-4), w), dashboardPanel("Completed per project", chart(names, values, w-4), w)))
	} else {
		b.WriteString(dashboardPanel(chartTitle, chart(daily, counts, width-4), width) + "\n" + dashboardPanel("Completed per project", chart(names, values, width-4), width))
	}

	// Keep the summary fixed and reserve at least three rows for browsing tasks.
	summary := strings.Split(b.String(), "\n")
	summaryLimit := max(1, m.height-11)
	if len(summary) > summaryLimit {
		summary = summary[:summaryLimit]
	}
	for i, line := range summary {
		summary[i] = ansi.Truncate(line, width, "…")
	}
	rows := s.taskRows()
	visible := max(2, m.height-4-len(summary)-2)
	cursor := min(m.statsCursor, max(0, len(rows)-1))
	start := cursor
	used := 2 // Reserve the heading and the selected task.
	for start > 0 {
		cost := 2
		if statisticsTaskGroup(rows[start-1], m.statsPeriod) != statisticsTaskGroup(rows[start], m.statsPeriod) {
			cost++
		}
		if used+cost > visible {
			break
		}
		used += cost
		start--
	}
	lines := []string{strings.Join(summary, "\n"), titleStyle.Render("TASKS")}
	if len(rows) == 0 {
		lines = append(lines, dim.Render("Nothing here"))
	}
	end, used := start, 0
	previousGroup := ""
	for end < len(rows) {
		group := statisticsTaskGroup(rows[end], m.statsPeriod)
		cost := 2
		if group != previousGroup && end > start {
			cost++
		}
		if used+cost > visible {
			break
		}
		if group != previousGroup {
			background := "#bfdbfe"
			if group == "No due date" {
				background = "#d1d5db"
			}
			heading := taskGroupHeading(group, background, width)
			if end == start {
				heading = strings.TrimPrefix(heading, "\n")
			}
			lines = append(lines, heading)
		} else {
			lines = append(lines, dim.Render(strings.Repeat("─", max(1, width))))
		}
		lines = append(lines, m.statisticsTaskLine(rows[end], width, m.statsTasksFocus && !m.focusSide && end == cursor))
		previousGroup = group
		used += cost
		end++
	}
	lines = append(lines, dim.Render(fmt.Sprintf("%d–%d / %d · ↑/↓ tasks · enter view · ↑ first task: periods", min(start+1, len(rows)), end, len(rows))))
	return strings.Join(lines, "\n")
}
