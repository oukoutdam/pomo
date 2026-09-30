package pomo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPresetsAndCustom(t *testing.T) {
	for _, test := range []struct {
		name, input, focus, rest string
		every                    int
		sound                    bool
	}{
		{"classic", "1\ny\n", "25m", "5m", 4, true},
		{"deep", "2\nn\n", "50m", "10m", 4, false},
		{"custom", "3\n24\n6m\n18m\n0\nn\n", "24m0s", "6m0s", 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			var output bytes.Buffer
			if err := Configure(strings.NewReader(test.input), &output, dir, DefaultConfig()); err != nil {
				t.Fatal(err)
			}
			c, err := LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			if c.Focus != test.focus || c.Break != test.rest || c.LongEvery != test.every || c.Sound != test.sound {
				t.Fatalf("config mismatch: %+v", c)
			}
		})
	}
}

func TestConfigCancellationPreservesSettings(t *testing.T) {
	dir := t.TempDir()
	original := Config{"50m", "10m", "20m", 4, false}
	if err := SaveConfig(dir, original); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Configure(strings.NewReader("3\n24\n"), &output, dir, original); err == nil {
		t.Fatal("EOF accepted as complete config")
	}
	got, err := LoadConfig(dir)
	if err != nil || got != original {
		t.Fatal("cancelled wizard changed config")
	}
}

func TestConfigValidation(t *testing.T) {
	for _, value := range []string{"0", "-1m", "nonsense", "25h", "NaN", "500ms"} {
		if _, err := parseDuration(value); err == nil {
			t.Fatalf("accepted duration %q", value)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"focus":"oops"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("accepted malformed settings")
	}
}
