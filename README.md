# pomo

A terminal Pomodoro timer for macOS. It runs focus and break countdowns, lets
you extend a focus past the bell, and records focus time in a local SQLite file.
`pomo stats` prints totals and a calendar of daily focus time. There are no
accounts, tags, notes, or background service.

This project was vibecoded: the code was written with AI coding assistance, and
the author directed and tested it. It is a personal project. Bug reports and
fixes are welcome.

## Install

Clone or download this repository, then build with Go 1.25 or later:

```sh
cd pomo
make build
./bin/pomo
```

This produces one binary at `bin/pomo`. SQLite is embedded through a pure-Go
driver, so running it doesn't need Python, a C compiler, or the SQLite
command-line tool. Notifications and sound go through helpers that ship with
macOS. The binary only runs on the operating system and CPU architecture it was
built for.

To put it on your PATH, copy `bin/pomo` to a directory already on your PATH,
or use `go install .` from the repository. To install directly from GitHub, run
`go install github.com/oukoutdam/pomo@latest`; Go places the binary in your Go
bin directory.

## Usage

```sh
pomo                        # guided focus and break sessions
pomo focus --duration 50m   # override the first focus only
pomo break                 # start a standalone short break any time
pomo break --duration 10m   # choose this break's length
pomo --quiet               # visible timer + desktop banner, no sound
pomo --quiet --no-notify    # visible timer, no sound or desktop banner
pomo stats                 # hours, streaks, and the last 52 weeks
pomo stats --year 2026      # calendar for a particular year
pomo config                # presets and custom settings
pomo config --show         # settings and storage paths
```

Durations accept `24m`, `50m`, `1h`, or `30s`; a bare number means minutes.
The valid range is 1 second through 24 hours. Short durations are useful for
trying the controls. A `--duration` override applies only to the first timer.
Later timers in the same guided run use your saved configuration.

## Configuration

`pomo config` offers:

| Preset | Focus | Short break | Long break | Frequency |
| --- | --- | --- | --- | --- |
| Classic (default) | 25m | 5m | 15m | Every 4 completed focuses |
| Deep work | 50m | 10m | 20m | Every 4 completed focuses |
| Custom | Your choice | Your choice | Your choice | Your choice; 0 disables long breaks |

The wizard also asks whether to play sound. Pressing Enter keeps the displayed
value. If you leave the wizard before finishing, your saved settings don't
change.

`--quiet` overrides the sound preference for one run. `--no-notify` turns off
the desktop banner and is separate from sound, so pass both flags if you want
neither.

## Controls

After each focus, pomo waits for you to choose what happens next. Nothing starts
automatically.

| Key | Action |
| --- | --- |
| `p` or Space | Pause/resume a countdown or extension |
| `e` after focus completion | Extend this focus with a stopwatch |
| `b` during focus, while paused, after the bell, or in extension | Save focus and start the appropriate break |
| `f` during a break, while paused, or after the bell | End the break and start a new focus |
| Enter after focus completion | Start the appropriate break |
| Enter after break completion | Start the next focus |
| `q` or Ctrl+C | Save and quit |

The calendar counts elapsed focus time from both completed and interrupted
focuses. An extension stays attached to its original focus and counts as work,
but it doesn't add to the completed-session count a second time. Pauses, breaks,
and time spent deciding after the bell do not count. You can quit and run
`pomo break` later.

The long-break count applies to the current guided run and resets when you quit.
Only completed focus countdowns advance it. Stopping a focus early offers a
short break. A standalone `pomo break` uses the configured short-break length.

Timers need an interactive terminal and stay attached to it, so closing the
terminal stops the timer. A terminal multiplexer such as tmux can keep it
running. Stats work in pipes and redirected output.

## Timer screen

The timer is a rounded panel, centered near the top of the terminal:

```text
  pomo

  ╭──────────────────────────────────────────╮
  │  FOCUS                          Running  │
  │                                          │
  │                  18:42                   │
  │                                          │
  │  ━━━━━━━━━─────────────────────────────  │
  │  6m 18s focused                25m goal  │
  │                                          │
  │  Today  1h 20m        This week  6h 45m  │
  ╰──────────────────────────────────────────╯

  p/space pause   b break   q quit

  Sound on · Notifications on
```

Focus is green, breaks are blue, and a paused timer is amber. Every state also
has a text label. Colors adapt to light and dark terminal backgrounds. When a
focus completes, the clock shows `00:00` and “Focus complete”; extending starts
a `+MM:SS` stopwatch. Timers of an hour or more show hours. The controls line
lists only the keys that work in the current state. Warnings appear below the
controls, ahead of the routine hints. Pasted text is ignored.

The panel is at most 56 columns wide. Below 48 columns, metrics and controls
stack. Below 36 columns or 16 rows, the timer keeps only its state, clock,
essential controls, and any warning. Resizing takes effect immediately.

`NO_COLOR` disables color throughout the CLI. Help, stats, and configuration
are plain text when redirected or when `TERM=dumb`. Timers still require an
interactive terminal with cursor control.

## Stats

`pomo stats` shows hours for today, this week, this month, this year, and all
time, along with the completed focus count, current and best streaks, and daily
activity:

```text
  pomo / focus history

  ╭──────────────────────────────────────────────────────────────────────────╮
  │  FOCUS TIME                                                              │
  │                                                                          │
  │  Today                  This week              All time                  │
  │  2h 15m                 9h 40m                 116h 30m                  │
  │                                                                          │
  │  This month  28h 05m                                This year  116h 30m  │
  │                                                                          │
  │  ──────────────────────────────────────────────────────────────────────  │
  │                                                                          │
  │  214 completed focuses · 5 day streak · 12 day best                      │
  ╰──────────────────────────────────────────────────────────────────────────╯
```

Below the summary is the calendar, drawn as panels with month labels, weekday
rows, date ranges, and an intensity legend. In a color terminal, days with focus
are green squares and empty days are dots. With `NO_COLOR` set, a dumb terminal,
or redirected output, cells use `.`, `1`, `2`, `3`, `4` and the output contains
no ANSI escapes. The buckets are no focus, under 1 hour, 1–2 hours, 2–4 hours,
and 4+ hours.

Totals stack on narrow terminals, and very narrow reports omit the frames. The
calendar is split into panels of whole weeks, roughly equal in size, so no
history is dropped. Future dates stay blank. Reports print to normal terminal
scrollback and aren't interactive.

Weeks begin on Monday. Reports use your computer's current local timezone. A
focus that crosses midnight is split between the two days, and daylight saving
time is respected. Because of this, changing timezone can change which day an
old session is grouped under.

A streak day requires at least one minute of focus. A streak that ran through
yesterday still counts as current until the end of today. Durations below a
minute display as `<1m`. The `--year` flag changes the calendar and its total;
the summary still shows current periods and all-time totals.

## Data

By default macOS stores everything in:

```text
~/Library/Application Support/pomo/
  config.json       Saved timer preferences
  history.sqlite3   Sessions and active focus intervals
  timer.lock        Prevents overlapping timers
```

SQLite can also create `history.sqlite3-wal` and `history.sqlite3-shm` while the
database is open. The directory is private to your user. Set `POMO_HOME` to use
a different directory, which is also useful for isolated tests:

```sh
POMO_HOME=/tmp/my-pomo-test pomo focus --duration 5s --quiet --no-notify
POMO_HOME=/tmp/my-pomo-test pomo stats
```

Progress is committed about once a second and at state changes. An unclean exit
can lose up to about one second of recent work. Time while the app was not
running is never counted when you reopen it. A completed countdown keeps its
completed status even if the process crashes during an extension. SIGTERM,
SIGHUP, and Ctrl+C save and exit normally. Only one timer can run per data
directory, but you can view stats from another terminal while it runs.

A gap of more than three seconds between clock ticks pauses the timer and leaves
the gap out of the total. This covers the computer going to sleep. Press `p` to
resume. pomo does not watch for keyboard or mouse activity, so if you leave your
desk with the computer awake, the time still counts until you pause or stop.

For a backup, quit pomo and any stats command, then copy the entire data
directory, including SQLite sidecar files if present. Restore it while pomo is
closed.

## Notifications on macOS

When a focus or break completes, pomo sends a desktop notification that includes
progress information, and plays the macOS Glass sound. The completed session is
saved before either is attempted, so a failed notification or sound doesn't
change the saved result. Failures are shown on the timer screen. A bounded
worker sends notifications one at a time, so a slow helper doesn't block the
controls. Quitting cancels anything still pending.

Notifications use built-in AppleScript (`osascript`), and macOS may attribute
them to Script Editor or the script host. Enable notifications for that sender
in System Settings → Notifications. Focus / Do Not Disturb can suppress banners;
audio volume and output-device settings affect sound. A successful exit status
from the command doesn't guarantee that permission was granted or that a banner
appeared.

Linux/FreeBSD can run the timer and reports, but desktop notification/audio
support is macOS-only in this version. Windows interactive timers are not
supported yet. No extra desktop helper or app bundle is installed.

## Development

The code, tests, and documentation were developed with AI assistance. Changes
still need review and verification. Before submitting a contribution, explain
what changed, test the affected behavior, and run these checks:

```sh
make test
make check                 # go vet + race-enabled tests
make build                 # CGO_ENABLED=0 standalone binary
```

The timer state machine accepts explicit timestamps so accounting tests run
without waiting. Tests cover extension, pauses, interrupted sessions, break
cadence, clock gaps, checkpoint recovery, concurrent access, configuration,
notification ordering and cancellation, input closure, pasted text, responsive
layouts, monochrome output, calendar boundaries, and daylight saving.

Bubble Tea and Lip Gloss are pinned to v2.0.0 to keep the Go 1.25 minimum.
Bubble Tea handles terminal input, resizing, rendering, and restoring the
terminal on exit. Timing and storage stay in the engine and controller. The view
helpers are pure functions and share a small style layer with the static CLI
commands.

The source is split into the CLI and live screen, the timer state and
controller, configuration, SQLite storage, reporting, and a replaceable
notifier. Database schema version 1 is tracked with SQLite's `user_version`. An
older binary rejects a newer schema instead of modifying it.

## License

[MIT](LICENSE)
