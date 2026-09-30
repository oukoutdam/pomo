package pomo

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

const usage = `pomo - focus, take a break, see your progress

Usage:
  pomo                              Start a guided focus/break cycle
  pomo focus [--duration 50m]        Start focus with an optional duration
  pomo break [--duration 10m]        Start a break whenever you want
  pomo stats [--year 2026]           Focus totals and contribution calendar
  pomo config                       Choose a preset or custom rhythm
  pomo config --show                Show settings and storage paths

Timer flags:
  -q, --quiet       No sound (keep desktop notifications and timer)
  --no-notify       No desktop banner (use --quiet too for full silence)
  --duration 24m   Override the first timer only; seconds and hours also work

Timer keys:
  p / Space        Pause or resume
  e                Extend finished focus into a stopwatch
  b                Save focus and start a break, including during extension
  f                End a break and start focus, including while paused
  Enter            Start break after focus, or focus after a finished break
  q / Ctrl+C       Save and quit

Defaults: 25m focus, 5m break, 15m long break every 4 completed focuses.
Use POMO_HOME to override the data directory. No background process is started.
`

func Run(args []string, in, out, errOut *os.File) error {
	command := "focus"
	// Accept common timer flags on either side of the command.
	var quiet, noNotify bool
	var filtered []string
	for _, arg := range args {
		switch arg {
		case "-q", "--quiet":
			quiet = true
		case "--no-notify":
			noNotify = true
		default:
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			printHelp(out)
			return nil
		case "--version", "version":
			fmt.Fprintln(out, "pomo 0.1.0")
			return nil
		case "focus", "break", "stats", "config":
			command, args = args[0], args[1:]
		default:
			if len(args[0]) > 0 && args[0][0] != '-' {
				return fmt.Errorf("unknown command %q; run pomo --help", args[0])
			}
		}
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { printHelp(errOut) }
	var duration string
	var year int
	var show bool
	switch command {
	case "focus", "break":
		fs.StringVar(&duration, "duration", "", "Override the first timer duration")
	case "stats":
		fs.IntVar(&year, "year", 0, "Calendar year (1970-9999)")
	case "config":
		fs.BoolVar(&show, "show", false, "Show current settings")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; run pomo --help", fs.Arg(0))
	}
	if year != 0 && (year < 1970 || year > 9999) {
		return errors.New("year must be between 1970 and 9999")
	}
	dir, err := dataDirectory()
	if err != nil {
		return err
	}
	c, err := LoadConfig(dir)
	if err != nil {
		return err
	}
	if command == "config" {
		if show {
			ShowConfig(out, dir, c)
			return nil
		}
		return Configure(in, out, dir, c)
	}
	if command == "stats" {
		store, err := OpenStore(dir)
		if err != nil {
			return err
		}
		defer store.Close()
		now := time.Now()
		report, err := readReport(store, now)
		if err != nil {
			return err
		}
		PrintStats(out, report, now, year)
		return nil
	}
	if !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) || os.Getenv("TERM") == "dumb" {
		return errors.New("timers need an interactive terminal; run pomo in a terminal (pomo stats works with redirected output)")
	}
	value := c.Focus
	if command == "break" {
		value = c.Break
	}
	if duration != "" {
		value = duration
	}
	planned, err := parseDuration(value)
	if err != nil {
		return err
	}
	unlock, err := acquireTimerLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	store, err := OpenStore(dir)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Recover(); err != nil {
		return err
	}
	return RunTimer(in, out, store, c, command, planned, quiet, noNotify, NewDesktopNotifier())
}

func terminalWidth(out *os.File) int {
	width, _, err := term.GetSize(int(out.Fd()))
	if err != nil || width < 1 {
		return 80
	}
	return width
}
