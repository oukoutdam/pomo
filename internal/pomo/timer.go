package pomo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

type timerTick struct{}
type timerQuit struct{}
type notificationResult struct{ err error }

// Bubble Tea v2.0.0 ignores EOF. Preserve the file interface (and therefore
// cancellable reads and raw mode), but make closure end the program.
var errInputClosed = errors.New("timer input closed")

type timerInput struct{ *os.File }

func (in timerInput) Read(p []byte) (int, error) {
	n, err := in.File.Read(p)
	if errors.Is(err, io.EOF) {
		err = errInputClosed
	}
	return n, err
}

// The sole worker sends notifications in order. Its bounded queue never blocks
// Update, and cancellation interrupts outstanding desktop commands on shutdown.
type notificationWorker struct {
	events  chan Notification
	results chan notificationResult
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

func startNotificationWorker(notifier Notifier) *notificationWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &notificationWorker{make(chan Notification, 4), make(chan notificationResult, 4), ctx, cancel, make(chan struct{})}
	go func() {
		defer close(w.done)
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-w.events:
				if ctx.Err() != nil {
					return
				}
				err := notifier.Send(ctx, event)
				select {
				case w.results <- notificationResult{err}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return w
}
func (w *notificationWorker) stop() { w.cancel(); <-w.done }
func (w *notificationWorker) listen() tea.Msg {
	select {
	case result := <-w.results:
		return result
	case <-w.ctx.Done():
		return nil
	}
}

type timerModel struct {
	controller       *Controller
	report           Report
	worker           *notificationWorker
	now              func() time.Time
	lastSave         time.Time
	width, height    int
	color, dark      bool
	silent, noNotify bool
	notice, warning  string
	err              error
	stopped, pasting bool
}

func newTimerModel(c *Controller, worker *notificationWorker, quiet, noNotify, color bool, now time.Time) (*timerModel, error) {
	report, err := readReport(c.Store, now)
	if err != nil {
		return nil, err
	}
	notice := "Your time counts, including sessions you stop early."
	if c.Engine.Session.Kind == "break" {
		notice = "Rest time is not counted as focus."
	}
	return &timerModel{controller: c, worker: worker, report: report, now: time.Now,
		lastSave: now, width: 80, height: 24, color: color, dark: true,
		silent: quiet || !c.Config.Sound, noNotify: noNotify, notice: notice}, nil
}
func nextTimerTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return timerTick{} })
}
func (m *timerModel) Init() tea.Cmd {
	commands := []tea.Cmd{nextTimerTick(), m.worker.listen}
	if m.color {
		commands = append(commands, tea.RequestBackgroundColor)
	}
	return tea.Batch(commands...)
}
func (m *timerModel) checkpoint(now time.Time) error {
	if err := m.controller.Save(); err != nil {
		return err
	}
	report, err := readReport(m.controller.Store, now)
	if err != nil {
		return err
	}
	m.report, m.lastSave = report, now
	return nil
}
func (m *timerModel) stop(err error) tea.Cmd {
	if !m.stopped {
		m.err = errors.Join(m.err, err, m.controller.Stop())
		m.stopped = true
	}
	return tea.Quit
}
func (m *timerModel) completionNotification() {
	s := m.controller.Engine.Session
	title := "Focus complete"
	message := fmt.Sprintf("%s focused. Today: %s. Extend, take a break, or stop in pomo.", formatHours(s.Elapsed()), formatHours(m.report.Today))
	if s.Kind == "break" {
		title = "Break complete"
		message = fmt.Sprintf("Ready for another focus? Today: %s. Return to pomo when ready.", formatHours(m.report.Today))
	}
	select {
	case m.worker.events <- Notification{Title: "pomo - " + title, Message: message, Sound: !m.silent, Banner: !m.noNotify}:
	default:
		m.warning = "Notification queue busy; timer completion is saved."
	}
}
func (m *timerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.stopped {
		return m, nil
	}
	var next tea.Cmd
	key := ""
	quitting := false
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		return m, nil
	case tea.PasteStartMsg:
		m.pasting = true
		return m, nil
	case tea.PasteEndMsg:
		m.pasting = false
		return m, nil
	case tea.PasteMsg:
		return m, nil
	case tea.KeyPressMsg:
		if m.pasting {
			return m, nil
		}
		key = strings.ToLower(msg.String())
		quitting = key == "q" || key == "ctrl+c" || key == "ctrl+d"
	case timerQuit:
		quitting = true
	case timerTick:
		next = nextTimerTick()
	case notificationResult:
		if msg.err != nil {
			m.warning = "Notification unavailable; completion is saved. " + msg.err.Error()
		}
		return m, m.worker.listen
	default:
		return m, nil
	}
	// Use processing time, never the timestamp of a queued tick. This preserves
	// sleep detection even when a checkpoint or the event loop is delayed.
	now := m.now()
	c := m.controller
	completed, slept := c.Advance(now)
	changed := completed || slept
	if slept {
		m.warning = "Clock gap detected. Paused; press p to resume."
	}
	if completed {
		if err := m.checkpoint(now); err != nil {
			return m, m.stop(err)
		}
		m.notice = "No time is counted while you choose what comes next."
		m.completionNotification()
	}
	if quitting {
		return m, m.stop(nil)
	}
	before := c.Engine.Session.State
	switch key {
	case "p", "space", " ":
		c.Engine.TogglePause(now)
		if before != c.Engine.Session.State {
			changed = true
			if slept || strings.HasPrefix(m.warning, "Clock gap detected.") {
				m.warning = ""
			}
			m.notice = "Timer resumed."
			if c.Engine.Session.State == "paused" {
				m.notice = "Paused. Take your time; press p to resume."
			}
		}
	case "e":
		if c.Engine.Extend(now) {
			changed = true
			m.notice = "Extension counts as focus. Start a break whenever you are ready."
		}
	case "b", "f", "enter":
		if c.Engine.Session.Kind == "focus" && (key == "b" || key == "enter" && before == "waiting") {
			if err := c.StartBreak(now); err != nil {
				return m, m.stop(err)
			}
			changed = true
			m.notice = "Focus saved. Enjoy your break."
		} else if c.Engine.Session.Kind == "break" && (key == "f" || key == "enter" && before == "waiting") {
			if err := c.StartFocus(now); err != nil {
				return m, m.stop(err)
			}
			changed = true
			m.notice = "A fresh focus. One session at a time."
		}
	}
	if c.Engine.Session.State != "paused" && strings.HasPrefix(m.warning, "Clock gap detected.") {
		m.warning = ""
	}
	if changed || now.Sub(m.lastSave) >= time.Second {
		if err := m.checkpoint(now); err != nil {
			return m, m.stop(err)
		}
	}
	return m, next
}
func (m *timerModel) View() tea.View {
	v := tea.NewView(timerView(m.controller.Engine.Session, m.controller.LongBreak, m.report,
		m.notice, m.warning, m.width, m.height, m.silent, m.noNotify, newStyles(m.color, m.dark)))
	v.AltScreen = true
	return v
}

func RunTimer(in, out *os.File, store *Store, config Config, kind string, duration time.Duration, quiet, noNotify bool, notifier Notifier) error {
	// Register before starting the session. Bubble Tea's competing SIGINT/SIGTERM
	// handler is disabled; resize handling remains owned by Bubble Tea.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	now := time.Now()
	c, err := NewController(store, config, kind, duration, now)
	if err != nil {
		return err
	}
	worker := startNotificationWorker(notifier)
	defer worker.stop()
	m, err := newTimerModel(c, worker, quiet, noNotify, colorEnabled(out), now)
	if err != nil {
		return errors.Join(err, c.Stop())
	}
	options := []tea.ProgramOption{tea.WithInput(timerInput{in}), tea.WithOutput(out), tea.WithoutSignalHandler()}
	if !m.color {
		options = append(options, tea.WithColorProfile(colorprofile.NoTTY))
	}
	program := tea.NewProgram(m, options...)
	bridgeDone := make(chan struct{})
	bridgeStopped := make(chan struct{})
	go func() {
		defer close(bridgeStopped)
		select {
		case <-signals:
			program.Send(timerQuit{})
		case <-bridgeDone:
		}
	}()
	_, runErr := program.Run()
	close(bridgeDone)
	<-bridgeStopped
	worker.cancel()
	// Run has restored the terminal and stopped Update. Input closure, startup
	// failures, and renderer errors also get a final checkpoint without races.
	if !m.stopped {
		c.Advance(time.Now())
		m.stop(nil)
	}
	if errors.Is(runErr, errInputClosed) {
		runErr = nil
	}
	if err := errors.Join(runErr, m.err); err != nil {
		return err
	}
	report, err := readReport(store, time.Now())
	if err != nil {
		return err
	}
	styles := staticStyles(out)
	printWrapped(out, styles.heading.Render("Focus saved.")+" Today: "+formatHours(report.Today)+" | All time: "+formatHours(report.Total))
	printWrapped(out, "See your map: pomo stats\nStart a break any time: pomo break")
	return nil
}
