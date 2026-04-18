# Changelog

## 2026-04-18

### Session opener
- The coach now speaks a one-turn opener at session start (PTY mode): restates the `--goal` in its own words and cues the user to begin, or — when no goal is set — asks what they want to accomplish. Per-response instructions explicitly override the persona's default-silence rule so the model actually speaks.
- Fires exactly once, before the normal react/nudge cadence. Scenario mode is unchanged (still drives responses per scripted event).

### Cadence
- Shortened the idle-nudge timer from 15s to 5s so the coach pings a stalled user faster. React debounce (1.5s after shell activity) unchanged.

### `coach watch` sidecar
- New `coach watch` subcommand tails `logs/coach.log` in a separate pane/window and pretty-prints coach turns (bold cyan `coach> …`) and errors/warnings by default; `--all` shows every slog event. `--from-start` reads the whole file before following; `--no-color` disables ANSI codes; `--log PATH` overrides the log file. Non-JSON lines pass through verbatim.
- File-not-exist is polled (so you can start `coach watch` before `make run`); truncation/rotation is detected by comparing cursor position to file size and reopening from the top.
- New `make watch` target. Keeps the PTY clean — no overlays, no in-shell redraws — and composes with tmux / screen / second terminal windows.

### Lab framework — task corpus
- New `internal/lab` package: `Store` over `modernc.org/sqlite` (pure-Go, no cgo) opens `coach.db` and applies numbered migrations on open. Schema covers `tasks`, `experiments` (snapshots persona text + config), `ratings`, and `events` with FK cascade where appropriate.
- Seeded a 15-task starter corpus spanning beginner/intermediate/advanced and tagged (`navigation`, `files`, `git`, `network`, `dangerous`). Seeding is idempotent (`INSERT OR IGNORE` on `slug`).
- New `coach lab tasks {seed,list,show}` subcommands. `list` honors `--tag`; output is tab-aligned and ordered by difficulty then slug. `--db` on the `lab` group overrides the default `coach.db` path.
- Unit tests cover migration idempotence, seed idempotence, tag filtering, ordering, and `ErrTaskNotFound`.

## 2026-04-17

### Shell / PTY
- Wrap a real PTY-backed shell (was scripted scenarios only). Scripted mode remains opt-in via `--scenario`.
- Default wrapped shell is a clean `/bin/bash --noprofile --norc` with a minimal env — the host's starship / plugins / autosuggest never load in the coach session. `--shell` overrides.
- `BASH_SILENCE_DEPRECATION_WARNING=1` in the child env so the macOS bash 3.2 banner doesn't reach the agent.
- Custom `PS1`, `HISTFILE=/dev/null`, and `COACH=1` marker set for the child shell.
- ANSI + private-use / box-drawing / nerd-font glyph sanitization before forwarding output to the agent; suppress consecutive duplicate chunks.

### Agent + response cadence
- Decoupled context from response: `SendContext` silently adds to the conversation; new `TriggerResponse` is the only thing that fires `response.create`.
- PTY mode fires a react trigger 1.5s after shell activity settles (speak only on errors/typos/dangerous/off-goal; otherwise silent) and a nudge trigger after 15s of total idle.
- `TriggerResponse` is gated on a `speaking` flag — skips when the coach is still mid-response, eliminating the `conversation_already_has_active_response` error class.
- Session-level `max_response_output_tokens: 400` cap as a safety belt for runaway responses.

### Mic
- Mic muted while the coach is speaking (track `response.created` / `response.done`, drop user audio for ~800ms after done to let speakers drain). Kills the voice-feedback loop that caused whisper to hallucinate "user" utterances.

### Logging
- Replaced `log.Printf` with structured `log/slog` JSON handler to stderr; `logs/coach.log` is the single sink. `LOG_LEVEL` env var (debug|info|warn|error).
- Every entry tagged with `source` (`main`, `agent`, `coach`, …).
- Log the full persona at session init; every `SendContext` text; every `TriggerResponse` instructions set; every whisper transcript (so we can see what the mic thinks it heard).
- `make tail` pretty-prints with `jq` when available.

### Persona
- Hard 8-word-per-turn cap, explicit banned-openers list, default-silence rule.
- Three selectable skill levels — `beginner` (default, most protective), `intermediate` (light-touch), `advanced` (minimal intervention) — via `--persona`. Shared style block, diverging on WHEN to speak.
- `--goal "…"` feeds a SESSION GOAL into the persona; drift from it is a valid reason to speak.

### CLI
- New subcommand `coach dev voices` — plays a sample line in each Realtime voice; `--only <voice>` filter; interactive Enter-to-advance.

### Repo
- Resolved the AGENTS.md merge conflict introduced by an earlier stash.
- `skills-lock.json` gitignored; `*.db` gitignored for the upcoming lab storage.
