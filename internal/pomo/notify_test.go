package pomo

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestNotificationSoundAndBannerOptions(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS notification backend")
	}
	for _, test := range []struct {
		name          string
		sound, banner bool
		calls         int
	}{
		{"normal", true, true, 2}, {"quiet", false, true, 1},
		{"sound only", true, false, 1}, {"silent", false, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			n := &DesktopNotifier{run: func(ctx context.Context, command string, args ...string) error {
				calls = append(calls, command)
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("notification command has no timeout")
				}
				return nil
			}}
			if err := n.Send(context.Background(), Notification{"title", "message", test.sound, test.banner}); err != nil {
				t.Fatal(err)
			}
			if len(calls) != test.calls {
				t.Fatalf("calls = %v", calls)
			}
			if !test.sound && strings.Contains(strings.Join(calls, " "), "afplay") {
				t.Fatal("quiet played sound")
			}
		})
	}
}

func TestNotificationFailureStillAttemptsSoundAndUsesArguments(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS notification backend")
	}
	var calls int
	message := `Quotes " and shell $(stuff) are data`
	n := &DesktopNotifier{run: func(ctx context.Context, command string, args ...string) error {
		calls++
		if strings.HasSuffix(command, "osascript") {
			if args[len(args)-1] != message || strings.Contains(args[1], message) {
				t.Fatal("message interpolated into script")
			}
			return errors.New("notifications denied")
		}
		return nil
	}}
	if err := n.Send(context.Background(), Notification{"title", message, true, true}); err == nil {
		t.Fatal("notification error swallowed")
	}
	if calls != 2 {
		t.Fatal("banner failure prevented sound")
	}
}
