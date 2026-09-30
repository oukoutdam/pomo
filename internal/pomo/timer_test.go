package pomo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type notifierFunc func(context.Context, Notification) error

func (f notifierFunc) Send(ctx context.Context, n Notification) error { return f(ctx, n) }

func testTimerModel(t *testing.T, duration time.Duration) (*timerModel, func(time.Duration, tea.Msg) tea.Cmd) {
	t.Helper()
	now := testTime()
	c, err := NewController(testStore(t), Config{"2s", "1s", "3s", 2, true}, "focus", duration, now)
	if err != nil {
		t.Fatal(err)
	}
	// An unconsumed queue lets each test inspect the committed state before a
	// notification could be delivered, with no wall-clock timing dependency.
	w := &notificationWorker{events: make(chan Notification, 4)}
	m, err := newTimerModel(c, w, false, false, false, now)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return now }
	return m, func(after time.Duration, msg tea.Msg) tea.Cmd {
		now = testTime().Add(after)
		_, cmd := m.Update(msg)
		if m.err != nil {
			t.Fatal(m.err)
		}
		return cmd
	}
}
func press(code rune) tea.Msg { return tea.KeyPressMsg{Code: code} }
func assertState(t *testing.T, m *timerModel, state string, elapsed time.Duration) {
	t.Helper()
	s := m.controller.Engine.Session
	if s.State != state || s.Elapsed() != elapsed {
		t.Fatalf("want %s/%s, got %+v (%s)", state, elapsed, s, s.Elapsed())
	}
}
func TestModelPauseCompletionExtensionAndQuit(t *testing.T) {
	m, step := testTimerModel(t, 2*time.Second)
	step(time.Second, press('p'))
	assertState(t, m, "paused", time.Second)
	step(time.Minute, timerTick{})
	assertState(t, m, "paused", time.Second)
	step(time.Minute, press(' '))
	step(61*time.Second, timerTick{})
	assertState(t, m, "waiting", 2*time.Second)
	event := <-m.worker.events
	count, err := m.controller.Store.CompletedFocus()
	if err != nil || count != 1 || event.Title != "pomo - Focus complete" {
		t.Fatal("completion not committed before enqueue", count, err, event)
	}
	step(2*time.Minute, press('e'))
	step(121*time.Second, timerTick{})
	assertState(t, m, "overtime", 3*time.Second)
	step(122*time.Second, press('p'))
	step(3*time.Minute, press('p'))
	assertState(t, m, "overtime", 4*time.Second)
	cmd := step(181*time.Second, press('q'))
	assertState(t, m, "completed", 5*time.Second)
	// Persistence must already be complete before the returned quit command runs.
	report, err := readReport(m.controller.Store, testTime().Add(4*time.Minute))
	if err != nil || report.Total != 5*time.Second || report.Completed != 1 {
		t.Fatal(report, err)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("missing quit command")
	}
	if len(m.worker.events) != 0 {
		t.Fatal("extension sent duplicate notification")
	}
}
func TestModelGuidedCadenceAndEarlyBreak(t *testing.T) {
	m, step := testTimerModel(t, 2*time.Second)
	step(time.Second, press('b'))
	if m.controller.LongBreak || m.controller.Completed != 0 {
		t.Fatal("early stop advanced cadence")
	}
	step(2*time.Second, timerTick{})
	step(2*time.Second, press(tea.KeyEnter))
	step(3*time.Second, timerTick{})
	step(4*time.Second, timerTick{})
	step(4*time.Second, press(tea.KeyEnter))
	if m.controller.LongBreak {
		t.Fatal("first completed focus got long break")
	}
	step(5*time.Second, timerTick{})
	step(5*time.Second, press(tea.KeyEnter))
	step(6*time.Second, timerTick{})
	step(7*time.Second, timerTick{})
	step(7*time.Second, press('b'))
	if !m.controller.LongBreak || m.controller.Engine.Session.Planned != 3*time.Second || m.controller.Completed != 2 {
		t.Fatal("missing long break")
	}
	report, err := readReport(m.controller.Store, testTime().Add(7*time.Second))
	if err != nil || report.Total != 5*time.Second {
		t.Fatal(report, err)
	}
}
func TestModelClockGapAndPaste(t *testing.T) {
	m, step := testTimerModel(t, time.Minute)
	step(time.Second, timerTick{})
	step(time.Hour, timerTick{})
	assertState(t, m, "paused", time.Second)
	if !strings.Contains(m.warning, "Clock gap") {
		t.Fatal("no gap warning")
	}
	step(time.Hour, tea.PasteMsg{Content: "pbqe"})
	step(time.Hour, tea.PasteStartMsg{})
	step(time.Hour, press('q'))
	step(time.Hour, press('p'))
	step(time.Hour, tea.PasteEndMsg{})
	assertState(t, m, "paused", time.Second)
	step(time.Hour, press('p'))
	if m.warning != "" {
		t.Fatal("stale clock warning")
	}
	step(time.Hour+time.Second, timerTick{})
	assertState(t, m, "countdown", 2*time.Second)
}
func TestModelAllQuitPathsSave(t *testing.T) {
	for _, msg := range []tea.Msg{press('q'), press('Q'), tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, timerQuit{}} {
		m, step := testTimerModel(t, time.Minute)
		cmd := step(1500*time.Millisecond, msg)
		if cmd == nil || !m.stopped {
			t.Fatalf("did not quit: %v", msg)
		}
		report, err := readReport(m.controller.Store, testTime().Add(2*time.Second))
		if err != nil || report.Total != 1500*time.Millisecond {
			t.Fatal(report, err)
		}
	}
}
func TestModelSaveFailureDoesNotNotify(t *testing.T) {
	m, _ := testTimerModel(t, time.Second)
	m.now = func() time.Time { return testTime().Add(time.Second) }
	m.controller.Store.Close()
	_, cmd := m.Update(timerTick{})
	if m.err == nil || cmd == nil || !m.stopped || len(m.worker.events) != 0 {
		t.Fatal("save failure did not stop before notification")
	}
}
func TestNotificationWorkerSlowDeliveryAndCancellation(t *testing.T) {
	started := make(chan Notification, 1)
	cancelled := make(chan struct{})
	w := startNotificationWorker(notifierFunc(func(ctx context.Context, n Notification) error {
		started <- n
		<-ctx.Done()
		close(cancelled)
		return ctx.Err()
	}))
	defer w.stop()
	m, step := testTimerModel(t, time.Second)
	m.worker = w
	step(time.Second, timerTick{})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("notification never started")
	}
	count, err := m.controller.Store.CompletedFocus()
	if err != nil || count != 1 {
		t.Fatal("not saved before delivery")
	}
	// The notifier remains blocked while input completes and persists normally.
	step(time.Second, press('e'))
	step(2*time.Second, press('p'))
	assertState(t, m, "paused", 2*time.Second)
	step(2*time.Second, press('q'))
	w.cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel delivery")
	}
}
func TestNotificationFailureAndQueueBound(t *testing.T) {
	w := startNotificationWorker(notifierFunc(func(context.Context, Notification) error { return errors.New("delivery failed") }))
	defer w.stop()
	m, step := testTimerModel(t, time.Second)
	m.worker = w
	step(time.Second, timerTick{})
	select {
	case result := <-w.results:
		m.Update(result)
	case <-time.After(time.Second):
		t.Fatal("missing failure result")
	}
	step(time.Second, press('e'))
	if !strings.Contains(m.warning, "delivery failed") || !strings.Contains(m.View().Content, "Warning:") {
		t.Fatal("routine notice hid warning")
	}
	// A full queue must immediately return a warning, not block Update.
	m.worker = &notificationWorker{events: make(chan Notification, 1)}
	m.worker.events <- Notification{}
	m.completionNotification()
	if !strings.Contains(m.warning, "queue busy") {
		t.Fatal("no overflow warning")
	}
}
func TestNotificationWorkerSequential(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{})
	w := startNotificationWorker(notifierFunc(func(ctx context.Context, n Notification) error {
		started <- n.Title
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}))
	defer w.stop()
	w.events <- Notification{Title: "first"}
	w.events <- Notification{Title: "second"}
	if got := <-started; got != "first" {
		t.Fatal(got)
	}
	select {
	case <-started:
		t.Fatal("parallel delivery")
	default:
	}
	release <- struct{}{}
	select {
	case got := <-started:
		if got != "second" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("missing second notification")
	}
}
func TestTimerInputClosure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "empty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, err = (timerInput{f}).Read(make([]byte, 1))
	if !errors.Is(err, errInputClosed) || errors.Is(err, io.EOF) {
		t.Fatalf("EOF swallowed by Bubble Tea: %v", err)
	}
}

func TestRunTimerInputClosureSavesAndReturns(t *testing.T) {
	in, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	defer writer.Close()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	store := testStore(t)
	result := make(chan error, 1)
	go func() {
		result <- RunTimer(in, out, store, DefaultConfig(), "focus", time.Minute, true, true, notifierFunc(func(context.Context, Notification) error { return nil }))
	}()
	// Wait until the session is initialized; then close input without a quit key.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timer did not initialize")
		}
		time.Sleep(10 * time.Millisecond)
	}
	writer.Close()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("input closure did not stop timer")
	}
	var state string
	if err := store.db.QueryRow("SELECT state FROM sessions").Scan(&state); err != nil || state != "interrupted" {
		t.Fatal(state, err)
	}
	content, err := os.ReadFile(out.Name())
	if err != nil || !strings.Contains(string(content), "Focus saved.") {
		t.Fatal("missing saved summary", err)
	}
}

func TestRunTimerProgramErrorSavesBeforeReturning(t *testing.T) {
	in, err := os.CreateTemp(t.TempDir(), "closed-input")
	if err != nil {
		t.Fatal(err)
	}
	in.Close()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	store := testStore(t)
	err = RunTimer(in, out, store, DefaultConfig(), "focus", time.Minute, true, true, notifierFunc(func(context.Context, Notification) error { return nil }))
	if err == nil {
		t.Fatal("program error was ignored")
	}
	var state string
	if err := store.db.QueryRow("SELECT state FROM sessions").Scan(&state); err != nil || state != "interrupted" {
		t.Fatal(state, err)
	}
	content, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "Focus saved.") {
		t.Fatal("success summary printed after program error")
	}
}

func TestModelJumpFromBreakToFocus(t *testing.T) {
	for _, long := range []bool{false, true} {
		for _, state := range []string{"countdown", "paused", "waiting"} {
			for _, key := range []rune{'f', 'F'} {
				t.Run(fmt.Sprintf("long=%t/%s/%c", long, state, key), func(t *testing.T) {
					m, step := testTimerModel(t, 2*time.Second)
					c := m.controller
					if long {
						c.Completed = 1
					}
					step(2*time.Second, press('b'))
					if c.LongBreak != long {
						t.Fatal("wrong break type")
					}
					breakID, completed := c.Engine.Session.ID, c.Completed
					at := 2500 * time.Millisecond
					wantElapsed := 500 * time.Millisecond
					wantState := "interrupted"
					switch state {
					case "paused":
						step(2250*time.Millisecond, press('p'))
						wantElapsed = 250 * time.Millisecond
					case "waiting":
						at = 2*time.Second + c.Engine.Session.Planned
						step(at, timerTick{})
						wantElapsed = c.Engine.Session.Planned
						wantState = "completed"
					default:
						// Enter retains its completion-only behavior; pasting f does nothing.
						step(2250*time.Millisecond, press(tea.KeyEnter))
						step(2250*time.Millisecond, tea.PasteMsg{Content: "f"})
						if c.Engine.Session.ID != breakID {
							t.Fatal("break skipped without shortcut")
						}
					}
					notices := len(m.worker.events)
					step(at, press(key))
					assertState(t, m, "countdown", 0)
					if c.Engine.Session.Kind != "focus" || c.Engine.Session.Planned != 2*time.Second || c.LongBreak || c.Completed != completed {
						t.Fatal("focus did not start with the configured duration and unchanged cadence")
					}
					var savedState string
					var elapsed int64
					if err := c.Store.db.QueryRow("SELECT state FROM sessions WHERE id=?", breakID).Scan(&savedState); err != nil {
						t.Fatal(err)
					}
					if err := c.Store.db.QueryRow("SELECT SUM(end_ns-start_ns) FROM spans WHERE session_id=?", breakID).Scan(&elapsed); err != nil {
						t.Fatal(err)
					}
					if savedState != wantState || time.Duration(elapsed) != wantElapsed {
						t.Fatal("break not saved correctly", savedState, time.Duration(elapsed))
					}
					if len(m.worker.events) != notices {
						t.Fatal("skipping break sent a completion notification")
					}
					step(at+time.Second, timerTick{})
					if m.report.Total != 3*time.Second {
						t.Fatal("break or pause counted as focus", m.report.Total)
					}
				})
			}
		}
	}
}

func TestFocusShortcutDoesNotRestartFocus(t *testing.T) {
	m, step := testTimerModel(t, 2*time.Second)
	id := m.controller.Engine.Session.ID
	step(time.Second, press('f'))
	assertState(t, m, "countdown", time.Second)
	step(2*time.Second, press('f'))
	assertState(t, m, "waiting", 2*time.Second)
	if m.controller.Engine.Session.Kind != "focus" || m.controller.Engine.Session.ID != id {
		t.Fatal("f restarted focus or started a break")
	}
}
