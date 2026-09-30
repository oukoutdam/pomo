package pomo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRedirectedCommandsRemainPlain(t *testing.T) {
	t.Setenv("POMO_HOME", t.TempDir())
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CLICOLOR_FORCE", "1")
	for _, args := range [][]string{{"--help"}, {"focus", "--help"}, {"stats"}, {"config", "--show"}, {"config"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "input")
			if err := os.WriteFile(inputPath, []byte("1\ny\n"), 0600); err != nil {
				t.Fatal(err)
			}
			in, err := os.Open(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			out, err := os.Create(filepath.Join(dir, "output"))
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			if err := Run(args, in, out, out); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(out.Name())
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(content, '\x1b') {
				t.Fatal("redirected output has escapes")
			}
			if len(content) == 0 {
				t.Fatal("empty output")
			}
		})
	}
}
func TestLongConfigPathsWrapWithoutLosingText(t *testing.T) {
	dir := "/" + strings.Repeat("日本語🙂/", 30)
	var b bytes.Buffer
	ShowConfig(&b, dir, DefaultConfig())
	for _, line := range strings.Split(b.String(), "\n") {
		if ansi.StringWidth(line) > 80 {
			t.Fatal("path overflow", line)
		}
	}
	flattened := strings.ReplaceAll(b.String(), "\n", "")
	if !strings.Contains(flattened, filepath.Join(dir, "history.sqlite3")) {
		t.Fatal("path was lost")
	}
}
func TestTimerRejectsRedirectedInput(t *testing.T) {
	t.Setenv("POMO_HOME", t.TempDir())
	file, err := os.CreateTemp(t.TempDir(), "pipe")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := Run([]string{"focus"}, file, file, file); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatal(err)
	}
}
