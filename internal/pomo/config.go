package pomo

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Focus     string `json:"focus"`
	Break     string `json:"short_break"`
	LongBreak string `json:"long_break"`
	LongEvery int    `json:"long_break_every"`
	Sound     bool   `json:"sound"`
}

func DefaultConfig() Config { return Config{"25m", "5m", "15m", 4, true} }

func dataDirectory() (string, error) {
	if dir := os.Getenv("POMO_HOME"); dir != "" {
		return filepath.Abs(dir)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pomo"), nil
}

func parseDuration(s string) (time.Duration, error) {
	// Bare numbers in the configuration wizard mean minutes.
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		s += "m"
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < time.Second || d > 24*time.Hour {
		return 0, fmt.Errorf("duration %q must be between 1s and 24h (for example 24m)", s)
	}
	return d, nil
}

func (c Config) Validate() error {
	for _, s := range []string{c.Focus, c.Break, c.LongBreak} {
		if _, err := parseDuration(s); err != nil {
			return err
		}
	}
	if c.LongEvery < 0 || c.LongEvery > 100 {
		return errors.New("long_break_every must be 0 (disabled) or between 1 and 100")
	}
	return nil
}

func LoadConfig(dir string) (Config, error) {
	c := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("read config.json: %w", err)
	}
	return c, c.Validate()
}

func SaveConfig(dir string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "config.json"))
}

func ShowConfig(w io.Writer, dir string, c Config) {
	fmt.Fprintf(w, "Focus: %s\nShort break: %s\nLong break: %s\nLong break every: %d completed focus sessions (0 = disabled)\nSound: %t\nConfig: %s\nHistory: %s\n", c.Focus, c.Break, c.LongBreak, c.LongEvery, c.Sound, filepath.Join(dir, "config.json"), filepath.Join(dir, "history.sqlite3"))
}

func Configure(r io.Reader, w io.Writer, dir string, current Config) error {
	scanner := bufio.NewScanner(r)
	ask := func(label, fallback string) (string, error) {
		fmt.Fprintf(w, "%s [%s]: ", label, fallback)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", errors.New("configuration cancelled; settings were not changed")
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer == "" {
			answer = fallback
		}
		return answer, nil
	}
	fmt.Fprintln(w, "Choose your rhythm\n  1  Classic    25 / 5 / 15 minutes, long break every 4\n  2  Deep work  50 / 10 / 20 minutes, long break every 4\n  3  Custom    choose each duration and break frequency")
	c := current
	for {
		choice, err := ask("Preset (1, 2, or 3)", "3")
		if err != nil {
			return err
		}
		if choice == "1" {
			c = DefaultConfig()
			break
		}
		if choice == "2" {
			c = Config{"50m", "10m", "20m", 4, true}
			break
		}
		if choice != "3" {
			fmt.Fprintln(w, "Please enter 1, 2, or 3.")
			continue
		}
		for _, field := range []struct {
			label string
			value *string
		}{
			{"Focus duration (minutes or e.g. 24m)", &c.Focus},
			{"Short break duration", &c.Break},
			{"Long break duration", &c.LongBreak},
		} {
			for {
				value, err := ask(field.label, *field.value)
				if err != nil {
					return err
				}
				d, err := parseDuration(value)
				if err != nil {
					fmt.Fprintln(w, err)
					continue
				}
				*field.value = d.String()
				break
			}
		}
		for {
			value, err := ask("Long break every N completed focuses (0 = disabled)", strconv.Itoa(c.LongEvery))
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 100 {
				fmt.Fprintln(w, "Enter a number from 0 to 100.")
				continue
			}
			c.LongEvery = n
			break
		}
		break
	}
	defaultSound := "y"
	if !current.Sound {
		defaultSound = "n"
	}
	for {
		value, err := ask("Play a sound at completion? (y/n)", defaultSound)
		if err != nil {
			return err
		}
		switch strings.ToLower(value) {
		case "y", "yes":
			c.Sound = true
		case "n", "no":
			c.Sound = false
		default:
			fmt.Fprintln(w, "Enter y or n.")
			continue
		}
		break
	}
	if err := SaveConfig(dir, c); err != nil {
		return err
	}
	fmt.Fprintln(w, "\nSaved your rhythm.")
	ShowConfig(w, dir, c)
	return nil
}
