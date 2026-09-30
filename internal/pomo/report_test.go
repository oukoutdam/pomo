package pomo

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestReportSplitsCalendarBoundaries(t *testing.T) {
	loc := time.FixedZone("JST", 9*60*60)
	now := time.Date(2026, 1, 5, 12, 0, 0, 0, loc) // Monday
	spans := []Span{
		{time.Date(2025, 12, 31, 23, 30, 0, 0, loc), time.Date(2026, 1, 1, 0, 30, 0, 0, loc)},
		{time.Date(2026, 1, 4, 23, 45, 0, 0, loc), time.Date(2026, 1, 5, 0, 15, 0, 0, loc)},
	}
	r := BuildReport(spans, 2, now)
	if r.Today != 15*time.Minute || r.Week != 15*time.Minute || r.Month != time.Hour || r.Year != time.Hour || r.Total != 90*time.Minute {
		t.Fatalf("incorrect buckets: %+v", r)
	}
	if r.Days["2025-12-31"] != 30*time.Minute || r.Days["2026-01-01"] != 30*time.Minute {
		t.Fatal("midnight split failed")
	}
	if r.CurrentStreak != 2 || r.LongestStreak != 2 {
		t.Fatalf("streak: %+v", r)
	}
}

func TestReportDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		month      time.Month
		day, hours int
	}{{time.March, 8, 23}, {time.November, 1, 25}} {
		start := time.Date(2026, test.month, test.day, 0, 0, 0, 0, loc)
		end := start.AddDate(0, 0, 1)
		r := BuildReport([]Span{{start, end}}, 1, end)
		if r.Total != time.Duration(test.hours)*time.Hour || len(r.Days) != 1 {
			t.Fatalf("DST report: %+v", r)
		}
	}
}

func TestStreakGraceAndThreshold(t *testing.T) {
	now := testTime()
	start := dayStart(now)
	spans := []Span{
		{start.AddDate(0, 0, -2), start.AddDate(0, 0, -2).Add(time.Minute)},
		{start.AddDate(0, 0, -1), start.AddDate(0, 0, -1).Add(time.Minute)},
		{start, start.Add(30 * time.Second)},
	}
	r := BuildReport(spans, 1, now)
	if r.CurrentStreak != 2 || r.LongestStreak != 2 {
		t.Fatal("today grace failed")
	}
	spans = append(spans, Span{start.Add(time.Hour), start.Add(time.Hour + 30*time.Second)})
	r = BuildReport(spans, 1, now)
	if r.CurrentStreak != 3 || r.LongestStreak != 3 {
		t.Fatal("daily duration threshold not combined")
	}
	r = BuildReport(spans, 1, now.AddDate(0, 0, 2))
	if r.CurrentStreak != 0 {
		t.Fatal("missed day did not reset streak")
	}
}

func TestCalendarNarrowMonochromeAndLeapDay(t *testing.T) {
	now := time.Date(2024, 12, 31, 12, 0, 0, 0, time.UTC)
	r := Report{Days: map[string]time.Duration{"2024-02-29": 2 * time.Hour}}
	var b bytes.Buffer
	PrintCalendar(&b, r, now, 2024, 40, false)
	output := b.String()
	if strings.Contains(output, "\x1b") {
		t.Fatal("monochrome output contains ANSI")
	}
	if !strings.Contains(output, "3 ") || strings.Count(output, "Mon ") < 3 {
		t.Fatal("missing leap day or narrow panels")
	}
	// All 366 in-range days must be represented exactly once.
	cells := 0
	for _, line := range strings.Split(output, "\n") {
		if len(line) >= 6 && strings.Contains("Mon Tue Wed Thu Fri Sat Sun", strings.TrimSpace(line[:6])) {
			for _, char := range line[6:] {
				if strings.ContainsRune(".1234", char) {
					cells++
				}
			}
			if len(line) > 40 {
				t.Fatal("calendar row wider than terminal")
			}
		}
	}
	if cells != 366 {
		t.Fatalf("calendar has %d cells, want 366", cells)
	}
}

func TestCalendarEmptyAndColorLevels(t *testing.T) {
	r := BuildReport(nil, 0, testTime())
	if r.Total != 0 || r.CurrentStreak != 0 {
		t.Fatal("nonzero empty report")
	}
	for i, duration := range []time.Duration{0, time.Second, time.Hour, 2 * time.Hour, 4 * time.Hour} {
		if heatLevel(duration) != i {
			t.Fatalf("wrong color bucket %s", duration)
		}
	}
	var b bytes.Buffer
	PrintCalendar(&b, r, testTime(), 0, 120, true)
	if !strings.Contains(b.String(), "\x1b[48;5;") {
		t.Fatal("missing color")
	}
	if formatHours(time.Second) != "<1m" {
		t.Fatal("short focus rounded away")
	}
}

func TestCalendarMonthAfterShortInitialStub(t *testing.T) {
	var b bytes.Buffer
	PrintCalendar(&b, Report{Days: map[string]time.Duration{}}, testTime(), 0, 80, false)
	if !strings.Contains(b.String(), "Oct") {
		t.Fatal("short initial September stub hid October's label")
	}
}
