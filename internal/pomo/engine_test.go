package pomo

import (
	"testing"
	"time"
)

func testTime() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) }

func advanceSeconds(e *Engine, start time.Time, seconds int) {
	for i := 1; i <= seconds; i++ {
		e.Advance(start.Add(time.Duration(i) * time.Second))
	}
}

func TestFocusPauseCompleteExtend(t *testing.T) {
	now := testTime()
	e := NewEngine("focus", 5*time.Second, now)
	advanceSeconds(e, now, 2)
	e.TogglePause(now.Add(2 * time.Second))
	e.Advance(now.Add(time.Minute))
	if e.Session.Elapsed() != 2*time.Second {
		t.Fatal("pause counted as work")
	}
	e.TogglePause(now.Add(time.Minute))
	advanceSeconds(e, now.Add(time.Minute), 3)
	if !e.Session.Completed || e.Session.State != "waiting" || e.Session.Elapsed() != 5*time.Second {
		t.Fatalf("completion: %+v", e.Session)
	}
	if completed, _ := e.Advance(now.Add(5 * time.Minute)); completed {
		t.Fatal("duplicate completion event")
	}
	if e.Session.Elapsed() != 5*time.Second {
		t.Fatal("decision time counted")
	}
	if !e.Extend(now.Add(5 * time.Minute)) {
		t.Fatal("could not extend")
	}
	advanceSeconds(e, now.Add(5*time.Minute), 7)
	e.TogglePause(now.Add(5*time.Minute + 7*time.Second))
	e.Advance(now.Add(6 * time.Minute))
	e.TogglePause(now.Add(6 * time.Minute))
	advanceSeconds(e, now.Add(6*time.Minute), 3)
	e.Finish()
	if e.Session.Elapsed() != 15*time.Second || e.Session.State != "completed" {
		t.Fatalf("extension total: %+v", e.Session)
	}
	if len(e.Session.Spans) != 4 {
		t.Fatalf("want 4 active intervals, got %d", len(e.Session.Spans))
	}
}

func TestSleepAndBackwardClockPause(t *testing.T) {
	for _, gap := range []time.Duration{time.Hour, -time.Hour} {
		t.Run(gap.String(), func(t *testing.T) {
			now := testTime()
			e := NewEngine("focus", time.Minute, now)
			e.Advance(now.Add(time.Second))
			completed, slept := e.Advance(now.Add(gap))
			if completed || !slept || e.Session.State != "paused" {
				t.Fatal("clock gap did not pause")
			}
			if e.Session.Elapsed() != time.Second {
				t.Fatal("clock gap counted as work")
			}
			e.TogglePause(now.Add(gap))
			e.Advance(now.Add(gap + time.Second))
			if e.Session.Elapsed() != 2*time.Second {
				t.Fatal("resume failed")
			}
		})
	}
}

func TestCountdownCapsAtDeadline(t *testing.T) {
	now := testTime()
	e := NewEngine("focus", 1500*time.Millisecond, now)
	e.Advance(now.Add(time.Second))
	completed, _ := e.Advance(now.Add(2 * time.Second))
	if !completed || e.Session.Elapsed() != 1500*time.Millisecond {
		t.Fatal("countdown overcounted a late tick")
	}
	if e.Session.Spans[0].End != now.Add(1500*time.Millisecond) {
		t.Fatal("incorrect end timestamp")
	}
}

func TestInterruptedAndBreakBehavior(t *testing.T) {
	now := testTime()
	e := NewEngine("focus", time.Minute, now)
	e.Advance(now.Add(time.Second))
	if e.Extend(now) {
		t.Fatal("extended before completion")
	}
	e.Finish()
	if e.Session.State != "interrupted" || e.Session.Completed {
		t.Fatal("early stop marked complete")
	}
	b := NewEngine("break", time.Second, now)
	b.Advance(now.Add(time.Second))
	if b.Extend(now.Add(time.Second)) {
		t.Fatal("break can be extended as focus")
	}
}
