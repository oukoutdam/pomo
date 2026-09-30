package pomo

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type Report struct {
	Days                                    map[string]time.Duration
	Today, Week, Month, Year, Total         time.Duration
	Completed, CurrentStreak, LongestStreak int
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func monday(t time.Time) time.Time {
	t = dayStart(t)
	return t.AddDate(0, 0, -(int(t.Weekday())+6)%7)
}

func BuildReport(spans []Span, completed int, now time.Time) Report {
	r := Report{Days: make(map[string]time.Duration), Completed: completed}
	for _, span := range spans {
		start, end := span.Start.In(now.Location()), span.End.In(now.Location())
		if end.After(now) {
			end = now
		}
		for start.Before(end) {
			next := dayStart(start).AddDate(0, 0, 1)
			if next.After(end) {
				next = end
			}
			r.Days[start.Format(time.DateOnly)] += next.Sub(start)
			start = next
		}
	}
	today := dayStart(now)
	week := monday(now).Format(time.DateOnly)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format(time.DateOnly)
	year := fmt.Sprintf("%04d-01-01", now.Year())
	var active []string
	for date, duration := range r.Days {
		r.Total += duration
		if date == today.Format(time.DateOnly) {
			r.Today += duration
		}
		if date >= week {
			r.Week += duration
		}
		if date >= month {
			r.Month += duration
		}
		if date >= year {
			r.Year += duration
		}
		if duration >= time.Minute {
			active = append(active, date)
		}
	}
	sort.Strings(active)
	var previous time.Time
	run := 0
	for _, date := range active {
		current, _ := time.ParseInLocation(time.DateOnly, date, now.Location())
		if !previous.IsZero() && previous.AddDate(0, 0, 1).Equal(current) {
			run++
		} else {
			run = 1
		}
		if run > r.LongestStreak {
			r.LongestStreak = run
		}
		previous = current
	}
	cursor := today
	if r.Days[cursor.Format(time.DateOnly)] < time.Minute {
		cursor = cursor.AddDate(0, 0, -1)
	}
	for r.Days[cursor.Format(time.DateOnly)] >= time.Minute {
		r.CurrentStreak++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return r
}

func readReport(store *Store, now time.Time) (Report, error) {
	spans, err := store.FocusSpans()
	if err != nil {
		return Report{}, err
	}
	count, err := store.CompletedFocus()
	if err != nil {
		return Report{}, err
	}
	return BuildReport(spans, count, now), nil
}

func formatHours(d time.Duration) string {
	if d > 0 && d < time.Minute {
		return "<1m"
	}
	minutes := int(d / time.Minute)
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

func PrintTotals(w io.Writer, r Report) {
	fmt.Fprintln(w, "FOCUS HOURS")
	for _, item := range []struct {
		label string
		value time.Duration
	}{
		{"Today", r.Today}, {"This week", r.Week}, {"This month", r.Month},
		{"This year", r.Year}, {"All time", r.Total},
	} {
		fmt.Fprintf(w, "  %-12s %s\n", item.label, formatHours(item.value))
	}
	fmt.Fprintf(w, "\nCompleted focuses: %d\nCurrent streak: %d days | Best: %d days\n", r.Completed, r.CurrentStreak, r.LongestStreak)
}

func heatLevel(d time.Duration) int {
	switch {
	case d <= 0:
		return 0
	case d < time.Hour:
		return 1
	case d < 2*time.Hour:
		return 2
	case d < 4*time.Hour:
		return 3
	default:
		return 4
	}
}

func heatCell(d time.Duration, color bool) string {
	level := heatLevel(d)
	if !color {
		return []string{". ", "1 ", "2 ", "3 ", "4 "}[level]
	}
	return fmt.Sprintf("\x1b[48;5;%dm  \x1b[0m", []int{236, 22, 28, 34, 46}[level])
}

// PrintCalendar uses whole-week panels instead of dropping old data on narrow screens.
func PrintCalendar(w io.Writer, r Report, now time.Time, year, width int, color bool) {
	start := monday(now).AddDate(0, 0, -51*7)
	end := dayStart(now)
	if year != 0 {
		start = time.Date(year, 1, 1, 0, 0, 0, 0, now.Location())
		end = time.Date(year, 12, 31, 0, 0, 0, 0, now.Location())
	}
	var calendarTotal time.Duration
	for date, duration := range r.Days {
		if date >= start.Format(time.DateOnly) && date <= end.Format(time.DateOnly) {
			calendarTotal += duration
		}
	}
	fmt.Fprintf(w, "\nFOCUS CALENDAR\n%s to %s\nCalendar total: %s\n", start.Format(time.DateOnly), end.Format(time.DateOnly), formatHours(calendarTotal))
	columns := (width - 6) / 2
	if columns < 1 {
		columns = 1
	}
	if columns > 53 {
		columns = 53
	}
	firstWeek, lastWeek := monday(start), monday(end)
	var weeks []time.Time
	for date := firstWeek; !date.After(lastWeek); date = date.AddDate(0, 0, 7) {
		weeks = append(weeks, date)
	}
	for offset := 0; offset < len(weeks); offset += columns {
		stop := offset + columns
		if stop > len(weeks) {
			stop = len(weeks)
		}
		panel := weeks[offset:stop]
		panelEnd := panel[len(panel)-1].AddDate(0, 0, 6)
		fmt.Fprintf(w, "\n  %s - %s\n", panel[0].Format("02 Jan 2006"), panelEnd.Format("02 Jan 2006"))
		labels := []byte(strings.Repeat(" ", len(panel)*2))
		lastLabelStart, lastLabelEnd := -1, -1
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
				pos := col * 2
				label := date.Format("Jan")
				if pos+len(label) <= len(labels) {
					// A short first-month stub must not hide the next month's name.
					if lastLabelStart >= 0 && pos < lastLabelEnd {
						for i := lastLabelStart; i < lastLabelEnd; i++ {
							labels[i] = ' '
						}
					}
					copy(labels[pos:], label)
					lastLabelStart = pos
					lastLabelEnd = pos + len(label)
				}
			}
		}
		fmt.Fprintf(w, "      %s\n", strings.TrimRight(string(labels), " "))
		for row, name := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
			fmt.Fprintf(w, "  %s ", name)
			for _, week := range panel {
				date := week.AddDate(0, 0, row)
				if date.Before(start) || date.After(end) || date.After(now) {
					fmt.Fprint(w, "  ")
					continue
				}
				fmt.Fprint(w, heatCell(r.Days[date.Format(time.DateOnly)], color))
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w, "\nDaily focus (hours):")
	for i, item := range []struct {
		duration time.Duration
		label    string
	}{
		{0, "none"}, {time.Minute, "<1h"}, {time.Hour, "1-2h"}, {2 * time.Hour, "2-4h"}, {4 * time.Hour, "4h+"},
	} {
		fmt.Fprintf(w, "  %s %s", heatCell(item.duration, color), item.label)
		if width < 65 || i == 4 {
			fmt.Fprintln(w)
		}
	}
}
