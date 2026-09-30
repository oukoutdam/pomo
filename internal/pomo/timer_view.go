package pomo

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func clockText(d time.Duration) string {
	d = max(0, d)
	seconds := int((d + time.Second - 1) / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}
func durationText(d time.Duration) string {
	seconds := int(max(0, d) / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%dh %02dm", seconds/3600, seconds/60%60)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
}
func progressBar(elapsed, planned time.Duration, width int) string {
	width = max(0, width)
	fraction := 0.0
	if planned > 0 {
		fraction = min(1.0, max(0.0, float64(elapsed)/float64(planned)))
	}
	filled := int(fraction * float64(width))
	return strings.Repeat("━", filled) + strings.Repeat("─", width-filled)
}
func timerLabels(s Session, long bool) (label, status, clock string) {
	label, status = "FOCUS", "Running"
	if s.Kind == "break" {
		label = "BREAK"
		if long {
			label = "LONG BREAK"
		}
	}
	clock = clockText(s.Planned - s.Elapsed())
	switch s.State {
	case "paused":
		status = "Paused"
	case "overtime":
		status = "Extending"
	case "waiting":
		status, clock = "Focus complete", "00:00"
		if s.Kind == "break" {
			status = "Break complete"
		}
	}
	if s.State == "overtime" || s.State == "paused" && s.Completed {
		clock = "+" + clockText(s.Elapsed()-s.Planned)
	}
	return
}
func timerControls(s Session) []string {
	if s.State == "waiting" {
		if s.Kind == "focus" {
			return []string{"e extend", "b/enter break", "q quit"}
		}
		return []string{"f/enter focus", "q quit"}
	}
	action := "pause"
	if s.State == "paused" {
		action = "resume"
	}
	keys := []string{"p/space " + action}
	if s.Kind == "focus" {
		keys = append(keys, "b break")
	} else {
		keys = append(keys, "f focus")
	}
	return append(keys, "q quit")
}
func spaced(left, right string, width int) string {
	return left + strings.Repeat(" ", max(1, width-ansi.StringWidth(left)-ansi.StringWidth(right))) + right
}
func centered(s string, width int) string {
	return strings.Repeat(" ", max(0, (width-ansi.StringWidth(s))/2)) + s
}

// timerView is deliberately pure: no clock reads, database access, or mutation.
func timerView(s Session, long bool, report Report, notice, warning string, width, height int, silent, noNotify bool, st styles) string {
	width, height = max(1, width), max(1, height)
	compact, stacked := width < 36 || height < 16, width < 48
	panelWidth := min(56, width-4)
	if compact {
		panelWidth = min(56, width)
	}
	inner := max(1, panelWidth-6)
	label, status, clock := timerLabels(s, long)
	accent := st.focus
	if s.Kind == "break" {
		accent = st.rest
	}
	if s.State == "paused" {
		accent = st.warning
	}
	keys := timerControls(s)
	var lines []string
	if compact {
		lines = append(lines, wrapWords(accent.Render(label+" · "+status), panelWidth)...)
		lines = append(lines, st.heading.Render(clock))
		for _, key := range keys {
			lines = append(lines, wrapWords(key, panelWidth)...)
		}
	} else {
		body := []string{spaced(accent.Render(label), accent.Render(status), inner)}
		if !stacked {
			body = append(body, "")
		}
		body = append(body, centered(st.heading.Render(clock), inner))
		if !stacked {
			body = append(body, "")
		}
		body = append(body, accent.Render(progressBar(s.Elapsed(), s.Planned, inner)))
		elapsed := durationText(s.Elapsed()) + " focused"
		if s.Kind == "break" {
			elapsed = durationText(s.Elapsed()) + " resting"
		}
		goal := durationText(s.Planned) + " goal"
		today, week := "Today  "+formatHours(report.Today), "This week  "+formatHours(report.Week)
		if stacked {
			body = append(body, elapsed, goal, today, week)
		} else {
			body = append(body, spaced(elapsed, goal, inner), "", spaced(today, week, inner))
		}
		panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2).Width(panelWidth).Render(strings.Join(body, "\n"))
		lines = append(lines, st.heading.Render("pomo"), "")
		lines = append(lines, strings.Split(panel, "\n")...)
		if stacked {
			lines = append(lines, keys...)
		} else {
			lines = append(lines, "", strings.Join(keys, "   "))
		}
	}
	// Warnings have priority over secondary details and routine guidance. Limit
	// their height, but keep wrapped text readable and visibly mark truncation.
	available := height - len(lines)
	if warning != "" && available > 0 {
		warningLines := wrapWords("Warning: "+warning, panelWidth)
		if len(warningLines) > available {
			warningLines = warningLines[:available]
			warningLines[available-1] = ansi.Truncate(warningLines[available-1], panelWidth-1, "") + "…"
		}
		for _, line := range warningLines {
			lines = append(lines, st.warning.Render(line))
		}
	}
	if !compact {
		secondary := []string{"", st.muted.Render("Sound " + onOff(!silent) + " · Notifications " + onOff(!noNotify))}
		if warning == "" {
			secondary = append(secondary, wrapWords(notice, panelWidth)...)
		}
		for _, line := range secondary {
			if len(lines) >= height {
				break
			}
			lines = append(lines, line)
		}
	}
	// Center the block near the top, measuring terminal cells rather than bytes.
	indent := strings.Repeat(" ", max(0, (width-panelWidth)/2))
	for i := range lines {
		lines[i] = indent + ansi.Truncate(lines[i], panelWidth, "…")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
