package pomo

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTimerViewStatesAndSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {48, 20}, {47, 20}, {36, 16}, {30, 12}, {80, 12}} {
		for _, state := range []string{"countdown", "paused", "waiting", "overtime"} {
			for _, kind := range []string{"focus", "break"} {
				for _, colors := range []bool{false, true} {
					t.Run(fmt.Sprintf("%dx%d/%s/%s/color=%t", size[0], size[1], kind, state, colors), func(t *testing.T) {
						s := Session{Kind: kind, State: state, Planned: 2 * time.Hour}
						if state == "overtime" {
							s.Completed = true
							s.Spans = []Span{{testTime(), testTime().Add(3 * time.Hour)}}
						}
						warning := "Notification unavailable. " + strings.Repeat("長いパス/", 30)
						text := timerView(s, false, Report{}, "routine guidance", warning, size[0], size[1], false, false, newStyles(colors, true))
						lines := strings.Split(text, "\n")
						if len(lines) > size[1] {
							t.Fatalf("too tall: %s", text)
						}
						for _, line := range lines {
							if ansi.StringWidth(line) > size[0] {
								t.Fatalf("too wide (%d): %s", ansi.StringWidth(line), text)
							}
						}
						if !colors && strings.Contains(text, "\x1b") {
							t.Fatal("monochrome view has ANSI")
						}
						for _, want := range []string{"Warning:", "q quit"} {
							if !strings.Contains(text, want) {
								t.Fatalf("missing %s: %s", want, text)
							}
						}
						if kind == "break" {
							want := "f focus"
							if state == "waiting" {
								want = "f/enter focus"
							}
							if !strings.Contains(text, want) {
								t.Fatalf("missing focus shortcut: %s", text)
							}
						}
						if strings.Contains(text, "routine guidance") {
							t.Fatal("warning hidden by guidance")
						}
						wantClock := "02:00:00"
						if state == "waiting" {
							wantClock = "00:00"
						}
						if state == "overtime" {
							wantClock = "+01:00:00"
						}
						if !strings.Contains(text, wantClock) {
							t.Fatal("clock missing", text)
						}
						compact := size[0] < 36 || size[1] < 16
						if compact == strings.Contains(text, "╭") {
							t.Fatal("wrong frame mode", text)
						}
						if compact && strings.Contains(text, "This week") {
							t.Fatal("secondary info in compact view")
						}
					})
				}
			}
		}
	}
}
func TestViewPurityThemeResizeAndControls(t *testing.T) {
	m, step := testTimerModel(t, time.Second)
	m.color = true
	m.Update(tea.BackgroundColorMsg{Color: color.White})
	light := m.View().Content
	m.Update(tea.BackgroundColorMsg{Color: color.Black})
	dark := m.View().Content
	if light == dark {
		t.Fatal("background did not adapt palette")
	}
	if !m.View().AltScreen {
		t.Fatal("missing alternate screen")
	}
	if m.controller.Engine.Session.Elapsed() != 0 {
		t.Fatal("view advanced timer")
	}
	step(time.Second, timerTick{})
	v := m.View().Content
	if !strings.Contains(v, "Focus complete") || !strings.Contains(v, "e extend") || strings.Contains(v, "p/space") {
		t.Fatal(v)
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	if strings.Contains(m.View().Content, "╭") {
		t.Fatal("resize did not simplify")
	}
	step(time.Second, press('e'))
	step(2*time.Second, press('p'))
	if v = m.View().Content; !strings.Contains(v, "+00:01") || !strings.Contains(v, "Paused") || !strings.Contains(v, "resume") {
		t.Fatal(v)
	}
}
func TestProgressClampAndDisplayWidth(t *testing.T) {
	for _, tc := range []struct {
		elapsed, planned time.Duration
		want             string
	}{
		{-time.Second, time.Second, "────"}, {0, 0, "────"}, {time.Second, 2 * time.Second, "━━──"}, {2 * time.Second, time.Second, "━━━━"},
	} {
		if got := progressBar(tc.elapsed, tc.planned, 4); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
	for _, line := range wrapWords(strings.Repeat("日本語/🙂é", 20), 12) {
		if ansi.StringWidth(line) > 12 {
			t.Fatal("wide word not wrapped", line)
		}
	}
	if clockText(24*time.Hour) != "24:00:00" {
		t.Fatal("hour clock")
	}
}
