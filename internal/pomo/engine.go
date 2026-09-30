package pomo

import "time"

type Span struct{ Start, End time.Time }

type Session struct {
	ID        int64
	Kind      string
	Started   time.Time
	Planned   time.Duration
	Completed bool
	State     string
	Spans     []Span
}

func (s Session) Elapsed() time.Duration {
	var total time.Duration
	for _, span := range s.Spans {
		total += span.End.Sub(span.Start)
	}
	return total
}

// Engine is the clock-independent state machine. Advance receives each clock tick.
type Engine struct {
	Session    Session
	last       time.Time
	resumeMode string
	openSpan   bool
}

func NewEngine(kind string, duration time.Duration, now time.Time) *Engine {
	return &Engine{Session: Session{Kind: kind, Started: now, Planned: duration, State: "countdown"}, last: now}
}

// Advance returns a completion event and a clock/sleep interruption event.
func (e *Engine) Advance(now time.Time) (completed, slept bool) {
	state := e.Session.State
	if state != "countdown" && state != "overtime" {
		e.last = now
		return false, false
	}
	// Wall time detects suspension even on systems whose monotonic clock stops in sleep.
	wallDelta := now.UnixNano() - e.last.UnixNano()
	if wallDelta < 0 || wallDelta > int64(3*time.Second) {
		e.resumeMode = state
		e.Session.State = "paused"
		e.openSpan = false
		e.last = now
		return false, true
	}
	delta := now.Sub(e.last)
	if delta < 0 {
		delta = 0
	}
	end := now
	if state == "countdown" {
		remaining := e.Session.Planned - e.Session.Elapsed()
		if delta >= remaining {
			end = e.last.Add(remaining)
			e.Session.Completed = true
			e.Session.State = "waiting"
			completed = true
		}
	}
	if end.After(e.last) {
		if e.openSpan {
			e.Session.Spans[len(e.Session.Spans)-1].End = end
		} else {
			e.Session.Spans = append(e.Session.Spans, Span{e.last, end})
			e.openSpan = true
		}
	}
	if completed {
		e.openSpan = false
	}
	e.last = now
	return completed, false
}

func (e *Engine) TogglePause(now time.Time) {
	switch e.Session.State {
	case "countdown", "overtime":
		e.resumeMode = e.Session.State
		e.Session.State = "paused"
		e.openSpan = false
	case "paused":
		e.Session.State = e.resumeMode
	}
	e.last = now
}

func (e *Engine) Extend(now time.Time) bool {
	if e.Session.Kind != "focus" || e.Session.State != "waiting" {
		return false
	}
	e.Session.State = "overtime"
	e.openSpan = false
	e.last = now
	return true
}

func (e *Engine) Finish() {
	if e.Session.Completed {
		e.Session.State = "completed"
	} else {
		e.Session.State = "interrupted"
	}
	e.openSpan = false
}

func nextBreak(c Config, completedCount int, justCompleted bool) (time.Duration, bool) {
	long := justCompleted && c.LongEvery > 0 && completedCount > 0 && completedCount%c.LongEvery == 0
	value := c.Break
	if long {
		value = c.LongBreak
	}
	d, _ := parseDuration(value)
	return d, long
}
