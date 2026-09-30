# pomo

A small terminal Pomodoro app for macOS. Focus, extend when you're in the flow,
take a break, and watch your focus history grow into a contribution calendar.
No accounts, tags, notes, or background service.

## Get started

Build with Go 1.25 or later:

```sh
cd pomo
make build
./bin/pomo
```

The result is one binary at `bin/pomo`. It embeds SQLite through a pure-Go driver;
Python, a C compiler, and the SQLite command-line tool are not needed at runtime.
macOS supplies the notification and audio helpers. The built binary is specific
to your operating system and CPU architecture.

To put it on your PATH, copy `bin/pomo` to a directory already on your PATH,
or use `go install .` and add your Go bin directory to PATH.

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
trying the controls. A `--duration` override applies only to the first timer;
subsequent guided sessions use your saved configuration.

## Your rhythm

`pomo config` offers:

| Preset | Focus | Short break | Long break | Frequency |
| --- | --- | --- | --- | --- |
| Classic (default) | 25m | 5m | 15m | Every 4 completed focuses |
| Deep work | 50m | 10m | 20m | Every 4 completed focuses |
| Custom | Your choice | Your choice | Your choice | Your choice; 0 disables long breaks |

The wizard also asks whether to play sound. Enter keeps the displayed value;
an incomplete wizard leaves your saved settings unchanged. `--quiet` overrides
the sound preference for this run. `--no-notify` controls the desktop banner
independently; use both flags if you want neither.

After each focus, pomo waits for your choice. Nothing starts automatically.

| Key | Action |
| --- | --- |
| `p` or Space | Pause/resume a countdown or extension |
| `e` after focus completion | Extend this focus with a stopwatch |
| `b` during focus, while paused, after the bell, or in extension | Save focus and start the appropriate break |
| Enter after focus completion | Start the appropriate break |
| Enter after break completion | Start the next focus |
| `q` or Ctrl+C | Save and quit |

The contribution calendar includes actual time from both completed and
interrupted focuses. An extension stays attached to its original focus and
counts as work, but doesn't increment the completed-session count again.
Pauses, breaks, and time spent deciding after the bell do not count. You can
always leave and run `pomo break` later.

Long-break cadence applies to the current guided run and resets when you quit.
Only completed focus countdowns advance it. Stopping a focus early offers a
short break. A standalone `pomo break` uses the configured short-break length.

Timers need an interactive terminal and stay attached to it. Stats work in
pipes and redirected output. Closing the terminal stops the timer; a terminal
multiplexer such as tmux can keep the foreground application running if needed.

## Your focus map

`pomo stats` shows today's, this week's, this month's, this year's, and all-time
hours; completed focus count; current/best streaks; and daily activity:

```text
FOCUS HOURS
  Today        2h 15m
  This week    9h 40m
  This month   28h 05m
  This year    116h 30m
  All time     116h 30m

Completed focuses: 214
Current streak: 5 days | Best: 12 days

      Sep
  Mon . 2 1 3
  Tue 1 3 2 2
  Wed 2 1 2 1
  Thu . 2 4 3
  Fri 1 2 1 2
  Sat . . 1
  Sun . . .
```

The example above is illustrative. In a color terminal the calendar uses five
shades of green/gray. With `NO_COLOR` set, a dumb terminal, or redirected output,
it uses `.`, `1`, `2`, `3`, `4` with a legend. The buckets are no focus, under
1 hour, 1–2 hours, 2–4 hours, and 4+ hours. Narrow terminals get multiple
whole-week panels so no history is dropped. Future dates are blank.

Weeks begin Monday. Reports use your computer's current local timezone, splitting
focus at midnight and respecting daylight saving time. Changing timezone can
therefore change the daily grouping of old sessions. A streak day requires at
least one minute of focus. Yesterday's streak remains current until the end of
today, giving you time to start. Durations below a minute display as `<1m`.
The `--year` flag changes the calendar and its total; the summary still shows
current periods and all-time totals.

## Data and reliability

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

Progress is committed approximately every second and at state changes. An
unclean exit can lose up to about one second of recent work; reopening never
counts time while the app was gone. A completed countdown keeps its completed
status even if the process crashes during extension. SIGTERM, SIGHUP, and Ctrl+C
save and exit normally. Only one timer can run per data directory, but you can
view stats from another terminal while it runs.

A clock/tick gap greater than three seconds (including detected computer sleep)
pauses the timer and excludes the gap. Press `p` when ready to resume. This is
a conservative gap detector, not an idle/keyboard-activity monitor: leaving your
desk with the computer awake still counts until you pause or stop.

For a backup, quit pomo and any stats command, then copy the entire data
directory, including SQLite sidecar files if present. Restore it while pomo is
closed. Existing `ampom` logs are untouched and not imported: they record no
duration and may include interrupted timers, so importing would invent hours.

## Notifications on macOS

Focus and break completion send a native desktop notification with useful
progress information. Sound uses macOS's Glass sound. Neither action changes
the saved timer result, and failures are shown in the timer screen.

Notifications use built-in AppleScript (`osascript`) and may be attributed to
Script Editor or the script host by macOS. Enable notifications for that sender
in System Settings → Notifications. Focus / Do Not Disturb can suppress banners;
audio volume and output-device settings affect sound. Notification permission
and a visible banner cannot be guaranteed by the command's exit status.

Linux/FreeBSD can run the timer and reports, but desktop notification/audio
support is macOS-only in this version. Windows interactive timers are not
supported yet. No extra desktop helper or app bundle is installed.

## Development

```sh
make test
make check                 # go vet + race-enabled tests
make build                 # CGO_ENABLED=0 standalone binary
```

The timer state machine accepts explicit timestamps so accounting tests run
without waiting. Tests cover extension, pauses, interrupted sessions, break
cadence, clock gaps, checkpoint recovery, concurrent access, configuration,
notification options, calendar boundaries, daylight saving, and narrow output.

Source is separated into the CLI and live screen, timer state/controller,
configuration, SQLite storage, reporting, and a replaceable notifier. Database
schema version 1 is tracked with SQLite's `user_version`; newer schemas are
rejected rather than modified by an older binary.
