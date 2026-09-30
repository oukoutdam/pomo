package pomo

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

type Controller struct {
	Engine    *Engine
	Store     *Store
	Config    Config
	Completed int
	LongBreak bool
}

func NewController(store *Store, config Config, kind string, duration time.Duration, now time.Time) (*Controller, error) {
	c := &Controller{Engine: NewEngine(kind, duration, now), Store: store, Config: config}
	return c, store.Save(&c.Engine.Session)
}

func (c *Controller) Advance(now time.Time) (bool, bool) {
	completed, slept := c.Engine.Advance(now)
	if completed && c.Engine.Session.Kind == "focus" {
		c.Completed++
	}
	return completed, slept
}

func (c *Controller) Save() error { return c.Store.Save(&c.Engine.Session) }

func (c *Controller) Stop() error {
	c.Engine.Finish()
	return c.Save()
}

func (c *Controller) StartBreak(now time.Time) error {
	if c.Engine.Session.Kind != "focus" {
		return nil
	}
	duration, long := nextBreak(c.Config, c.Completed, c.Engine.Session.Completed)
	if err := c.Stop(); err != nil {
		return err
	}
	c.Engine = NewEngine("break", duration, now)
	c.LongBreak = long
	return c.Save()
}

func (c *Controller) StartFocus(now time.Time) error {
	if c.Engine.Session.Kind != "break" || c.Engine.Session.State != "waiting" {
		return nil
	}
	if err := c.Stop(); err != nil {
		return err
	}
	duration, _ := parseDuration(c.Config.Focus)
	c.Engine = NewEngine("focus", duration, now)
	c.LongBreak = false
	return c.Save()
}

func RunTimer(in, out *os.File, store *Store, config Config, kind string, duration time.Duration, quiet, noNotify bool, notifier Notifier) error {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return err
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		fmt.Fprint(out, "\x1b[?25h\x1b[?1049l")
		term.Restore(int(in.Fd()), state)
		restored = true
	}
	defer restore()
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")
	err = interactiveTimer(in, out, store, config, kind, duration, quiet, noNotify, notifier)
	restore()
	if err != nil {
		return err
	}
	report, err := readReport(store, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Focus saved. Today: %s | All time: %s\nSee your map: pomo stats\nStart a break any time: pomo break\n", formatHours(report.Today), formatHours(report.Total))
	return nil
}

func interactiveTimer(in, out *os.File, store *Store, config Config, kind string, duration time.Duration, quiet, noNotify bool, notifier Notifier) error {
	c, err := NewController(store, config, kind, duration, time.Now())
	if err != nil {
		return err
	}
	keys := make(chan byte, 16)
	inputErrors := make(chan error, 1)
	inputDone := make(chan struct{})
	defer close(inputDone)
	go func() {
		buffer := make([]byte, 1)
		for {
			n, err := in.Read(buffer)
			if err != nil {
				inputErrors <- err
				return
			}
			if n > 0 {
				select {
				case keys <- buffer[0]:
				case <-inputDone:
					return
				}
			}
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	events := make(chan Notification, 4)
	noticeErrors := make(chan error, 8)
	noticeDone := make(chan struct{})
	go func() {
		defer close(noticeDone)
		for event := range events {
			if err := notifier.Send(context.Background(), event); err != nil {
				select {
				case noticeErrors <- err:
				default:
				}
			}
		}
	}()
	defer func() { close(events); <-noticeDone }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastSave := time.Now()
	lastDraw := time.Time{}
	lastScreen := ""
	report, err := readReport(store, time.Now())
	if err != nil {
		return err
	}
	notice := "Your time counts, including sessions you stop early."
	if kind == "break" {
		notice = "Rest time is not counted as focus."
	}
	for {
		var key byte
		quit := false
		select {
		case key = <-keys:
		case <-ticker.C:
		case <-signals:
			quit = true
		case inputErr := <-inputErrors:
			if err := c.Stop(); err != nil {
				return err
			}
			if inputErr == io.EOF {
				return nil
			}
			return inputErr
		case noticeErr := <-noticeErrors:
			notice = "Notification unavailable; focus is saved. " + noticeErr.Error()
		}
		now := time.Now()
		completed, slept := c.Advance(now)
		if slept {
			notice = "Clock gap detected. Paused; press p to resume."
		}
		changed := key != 0 || completed || slept
		if completed {
			notice = "No time is counted while you choose what comes next."
			// Commit completion before any external notification operation.
			if err := c.Save(); err != nil {
				return err
			}
			report, err = readReport(store, now)
			if err != nil {
				return err
			}
			title := "Focus complete"
			message := fmt.Sprintf("%s focused. Today: %s. Extend, take a break, or stop in pomo.", formatHours(c.Engine.Session.Elapsed()), formatHours(report.Today))
			if c.Engine.Session.Kind == "break" {
				title = "Break complete"
				message = fmt.Sprintf("Ready for another focus? Today: %s. Return to pomo when ready.", formatHours(report.Today))
			}
			select {
			case events <- Notification{Title: "pomo - " + title, Message: message, Sound: config.Sound && !quiet, Banner: !noNotify}:
			default:
				notice = "Notification queue busy; timer completion is saved."
			}
		}
		if key == 'q' || key == 'Q' || key == 3 || key == 4 || quit {
			return c.Stop()
		}
		switch key {
		case 'p', 'P', ' ':
			c.Engine.TogglePause(now)
			if c.Engine.Session.State == "paused" {
				notice = "Paused. Take your time; press p to resume."
			} else if c.Engine.Session.State != "waiting" {
				notice = "Timer resumed."
			}
		case 'e', 'E':
			if c.Engine.Extend(now) {
				notice = "Extension counts as focus. Start a break whenever you are ready."
			}
		case 'b', 'B':
			if c.Engine.Session.Kind == "focus" {
				if err := c.StartBreak(now); err != nil {
					return err
				}
				notice = "Focus saved. Enjoy your break."
			}
		case '\r', '\n':
			if c.Engine.Session.State == "waiting" {
				if c.Engine.Session.Kind == "focus" {
					err = c.StartBreak(now)
					notice = "Focus saved. Enjoy your break."
				} else {
					err = c.StartFocus(now)
					notice = "A fresh focus. One session at a time."
				}
				if err != nil {
					return err
				}
			}
		}
		if changed || now.Sub(lastSave) >= time.Second {
			if changed || c.Engine.Session.State == "countdown" || c.Engine.Session.State == "overtime" {
				if err := c.Save(); err != nil {
					return err
				}
			}
			report, err = readReport(store, now)
			if err != nil {
				return err
			}
			lastSave = now
		}
		if changed || now.Sub(lastDraw) >= time.Second {
			var screen strings.Builder
			renderTimer(&screen, c, report, notice, terminalWidth(out), quiet || !config.Sound, noNotify)
			if screen.String() != lastScreen {
				fmt.Fprint(out, screen.String())
				lastScreen = screen.String()
			}
			lastDraw = now
		}
	}
}

func clockText(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int((d + time.Second - 1) / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func renderTimer(out io.Writer, c *Controller, report Report, notice string, width int, silent, noNotify bool) {
	s := c.Engine.Session
	elapsed := s.Elapsed()
	label := "FOCUS"
	if s.Kind == "break" {
		label = "BREAK"
		if c.LongBreak {
			label = "LONG BREAK"
		}
	}
	status := s.State
	if s.State == "waiting" {
		status = "complete - waiting for you"
	}
	fmt.Fprint(out, "\x1b[H\x1b[2J")
	writeLines(out, "", "  pomo / "+label, "  "+status, "")
	display := clockText(s.Planned - elapsed)
	if s.State == "overtime" || (s.State == "paused" && s.Completed) {
		display = "+" + clockText(elapsed-s.Planned) + " extended"
	}
	if s.State == "waiting" {
		display = "Done. Take a breath."
	}
	writeLines(out, "  "+display, "")
	barWidth := width - 6
	if barWidth > 36 {
		barWidth = 36
	}
	if barWidth < 4 {
		barWidth = 4
	}
	filled := int(float64(elapsed) / float64(s.Planned) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	writeLines(out, "  ["+strings.Repeat("#", filled)+strings.Repeat("-", barWidth-filled)+"]", "")
	if s.Kind == "focus" {
		writeLines(out, "  This focus: "+formatHours(elapsed))
	}
	writeLines(out, "  Today: "+formatHours(report.Today), "  This week: "+formatHours(report.Week), "")
	if s.State == "waiting" {
		if s.Kind == "focus" {
			writeLines(out, "  e  Extend focus (stopwatch)", "  b / Enter  Start break", "  q  Stop and save")
		} else {
			writeLines(out, "  Enter  Start focus", "  q  Stop")
		}
	} else {
		writeLines(out, "  p / Space  Pause or resume")
		if s.Kind == "focus" {
			writeLines(out, "  b  Stop focus and start break")
		}
		writeLines(out, "  q / Ctrl+C  Save and quit")
	}
	writeLines(out, "", fmt.Sprintf("  Sound: %s | Notifications: %s", onOff(!silent), onOff(!noNotify)), "")
	// Wrap the informational line so common 80-column terminals do not scroll.
	lineWidth := width - 4
	if lineWidth < 10 {
		lineWidth = 10
	}
	for _, line := range wrapWords(notice, lineWidth) {
		writeLines(out, "  "+line)
	}
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func wrapWords(s string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if len(line)+1+len(word) > width && line != "" {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
