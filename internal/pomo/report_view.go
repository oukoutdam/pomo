package pomo

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Reports share the timer's frame and spacing, but remain ordinary scrollable
// output. No terminal queries or input handling are needed for static reports.
func PrintStats(w io.Writer, r Report, now time.Time, year int) {
	fmt.Fprintln(w, statsView(r, now, year, outputWidth(w), colorEnabled(w)))
}
func statsView(r Report, now time.Time, year, width int, color bool) string {
	width = max(1, width)
	st := reportStyles(color)
	margin := 0
	if width >= 36 {
		margin = 2
	}
	panelWidth := min(76, width-2*margin)
	parts := []string{st.heading.Render("pomo") + st.muted.Render(" / focus history"), "",
		totalsView(r, panelWidth, st), "", calendarView(r, now, year, panelWidth, color, st)}
	if r.Total == 0 {
		parts = append(parts, "", strings.Join(wrapWords("Your first focus starts the map. Run: pomo", panelWidth), "\n"))
	}
	content := strings.Join(parts, "\n")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", margin) + line
	}
	return strings.Join(lines, "\n")
}

func reportFrame(body string, width int) string {
	if width < 32 {
		return strings.Join(wrapWords(body, width), "\n")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2).Width(width).Render(body)
}
func reportInnerWidth(width int) int {
	if width < 32 {
		return max(1, width)
	}
	return width - 6
}
func PrintTotals(w io.Writer, r Report) {
	fmt.Fprintln(w, totalsView(r, min(76, outputWidth(w)), reportStyles(colorEnabled(w))))
}
func totalsView(r Report, width int, st styles) string {
	inner := reportInnerWidth(width)
	lines := []string{st.heading.Render(st.focus.Render("FOCUS TIME")), ""}
	if inner >= 56 {
		// Three equally sized columns give today's work and longer-term totals the
		// same hierarchy without making this a progress-to-goal chart.
		col := inner / 3
		row := func(values []string, style lipgloss.Style) string {
			var cells []string
			for _, v := range values {
				cells = append(cells, lipgloss.NewStyle().Width(col).Render(style.Render(v)))
			}
			return strings.TrimRight(strings.Join(cells, ""), " ")
		}
		lines = append(lines, row([]string{"Today", "This week", "All time"}, st.muted),
			row([]string{formatHours(r.Today), formatHours(r.Week), formatHours(r.Total)}, st.heading), "",
			spaced("This month  "+formatHours(r.Month), "This year  "+formatHours(r.Year), inner))
	} else {
		for _, item := range []struct {
			label string
			value time.Duration
		}{
			{"Today", r.Today}, {"This week", r.Week}, {"This month", r.Month}, {"This year", r.Year}, {"All time", r.Total},
		} {
			value := st.heading.Render(formatHours(item.value))
			lines = append(lines, spaced(item.label, value, inner))
		}
	}
	lines = append(lines, "", st.muted.Render(strings.Repeat("─", inner)), "")
	items := []string{fmt.Sprintf("%d completed focuses", r.Completed), fmt.Sprintf("%d day streak", r.CurrentStreak), fmt.Sprintf("%d day best", r.LongestStreak)}
	if ansi.StringWidth(strings.Join(items, " · ")) <= inner {
		lines = append(lines, strings.Join(items, " · "))
	} else {
		for _, item := range items {
			lines = append(lines, wrapWords(item, inner)...)
		}
	}

	return reportFrame(strings.Join(lines, "\n"), width)
}

func heatCell(d time.Duration, color bool) string {
	level := heatLevel(d)
	if !color {
		return []string{". ", "1 ", "2 ", "3 ", "4 "}[level]
	}
	if level == 0 {
		return reportStyles(true).muted.Render("· ")
	}
	// A one-cell square plus a gutter keeps adjacent days visually separate.
	return lipgloss.NewStyle().Foreground(lipgloss.Color([]string{"", "65", "71", "77", "114"}[level])).Render("■ ")
}
func PrintCalendar(w io.Writer, r Report, now time.Time, year, width int, color bool) {
	fmt.Fprintln(w, calendarView(r, now, year, max(1, width), color, reportStyles(color)))
}
func calendarView(r Report, now time.Time, year, width int, color bool, st styles) string {
	start, end := monday(now).AddDate(0, 0, -51*7), dayStart(now)
	if year != 0 {
		start = time.Date(year, 1, 1, 0, 0, 0, 0, now.Location())
		end = time.Date(year, 12, 31, 0, 0, 0, 0, now.Location())
	}
	var total time.Duration
	for date, d := range r.Days {
		if date >= start.Format(time.DateOnly) && date <= end.Format(time.DateOnly) {
			total += d
		}
	}
	lines := []string{st.heading.Render(st.focus.Render("FOCUS MAP"))}
	lines = append(lines, wrapWords(start.Format("02 Jan 2006")+" – "+end.Format("02 Jan 2006"), width)...)
	lines = append(lines, st.muted.Render("Calendar total  ")+st.heading.Render(formatHours(total)), "")
	var weeks []time.Time
	for week := monday(start); !week.After(monday(end)); week = week.AddDate(0, 0, 7) {
		weeks = append(weeks, week)
	}
	inner := reportInnerWidth(width)
	capacity := max(1, (inner-4)/2)
	// Balance the panels so the last one is not a small, orphaned strip.
	panels := (len(weeks) + capacity - 1) / capacity
	columns := (len(weeks) + panels - 1) / panels
	for offset := 0; offset < len(weeks); offset += columns {
		panel := weeks[offset:min(offset+columns, len(weeks))]
		panelStart, panelEnd := panel[0], panel[len(panel)-1].AddDate(0, 0, 6)
		if panelStart.Before(start) {
			panelStart = start
		}
		if panelEnd.After(end) {
			panelEnd = end
		}
		title := panelStart.Format("02 Jan 2006") + " – " + panelEnd.Format("02 Jan 2006")
		body := wrapWords(st.muted.Render(title), inner)
		body = append(body, "")
		gridWidth := 4 + 2*len(panel)
		indent := strings.Repeat(" ", max(0, (inner-gridWidth)/2))
		labels := calendarMonthLabels(panel)
		body = append(body, indent+"    "+st.muted.Render(labels))
		for row, name := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
			var cells strings.Builder
			cells.WriteString(indent + st.muted.Render(name) + " ")
			for _, week := range panel {
				date := week.AddDate(0, 0, row)
				if date.Before(start) || date.After(end) || date.After(now) {
					cells.WriteString("  ")
					continue
				}
				cells.WriteString(heatCell(r.Days[date.Format(time.DateOnly)], color))
			}
			body = append(body, cells.String())
		}
		lines = append(lines, reportFrame(strings.Join(body, "\n"), width), "")
	}
	legend := []string{}
	for _, item := range []struct {
		duration time.Duration
		label    string
	}{
		{0, "none"}, {time.Minute, "<1h"}, {time.Hour, "1–2h"}, {2 * time.Hour, "2–4h"}, {4 * time.Hour, "4h+"},
	} {
		legend = append(legend, heatCell(item.duration, color)+item.label)
	}
	lines = append(lines, st.muted.Render("Daily focus"))
	lines = append(lines, packReportItems(legend, width)...)
	// Header text and legends also respect terminal cell widths, including when
	// a terminal is too narrow for a frame. Calendar rows retain whole weeks.
	return strings.Join(wrapWords(strings.Join(lines, "\n"), width), "\n")
}
func calendarMonthLabels(panel []time.Time) string {
	labels := []byte(strings.Repeat(" ", len(panel)*2))
	lastStart, lastEnd := -1, -1
	for col, week := range panel {
		date := week
		for j := 0; j < 7; j++ {
			candidate := week.AddDate(0, 0, j)
			if candidate.Day() == 1 {
				date = candidate
				break
			}
		}
		if col == 0 || date.Day() == 1 {
			pos, label := col*2, date.Format("Jan")
			if pos+len(label) > len(labels) {
				continue
			}
			if lastStart >= 0 && pos < lastEnd {
				for i := lastStart; i < lastEnd; i++ {
					labels[i] = ' '
				}
			}
			copy(labels[pos:], label)
			lastStart, lastEnd = pos, pos+len(label)
		}
	}
	return strings.TrimRight(string(labels), " ")
}

// Keep a calendar swatch beside its label when the legend wraps.
func packReportItems(items []string, width int) []string {
	var lines []string
	line := ""
	for _, item := range items {
		if line != "" && ansi.StringWidth(line)+3+ansi.StringWidth(item) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += "   "
		}
		line += item
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
