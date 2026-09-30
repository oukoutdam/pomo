package pomo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Notification struct {
	Title, Message string
	Sound, Banner  bool
}

type Notifier interface {
	Send(context.Context, Notification) error
}

type commandRunner func(context.Context, string, ...string) error
type DesktopNotifier struct{ run commandRunner }

func NewDesktopNotifier() *DesktopNotifier {
	return &DesktopNotifier{run: func(ctx context.Context, command string, args ...string) error {
		output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w %s", command, err, strings.TrimSpace(string(output)))
		}
		return nil
	}}
}

func (n *DesktopNotifier) Send(ctx context.Context, event Notification) error {
	if !event.Banner && !event.Sound {
		return nil
	}
	if runtime.GOOS != "darwin" {
		return errors.New("desktop notifications and sound currently require macOS")
	}
	var failures []error
	if event.Banner {
		// Pass data as arguments: titles and messages never become AppleScript code.
		script := "on run argv\n display notification (item 2 of argv) with title (item 1 of argv)\nend run"
		cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := n.run(cmdCtx, "/usr/bin/osascript", "-e", script, event.Title, event.Message)
		cancel()
		if err != nil {
			failures = append(failures, err)
		}
	}
	if event.Sound {
		cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := n.run(cmdCtx, "/usr/bin/afplay", "/System/Library/Sounds/Glass.aiff")
		cancel()
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
