package pomo

import (
	"fmt"
	"sort"
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
