# coach

Live terminal coaching agent for non-technical users. `coach` wraps the user's shell in a PTY, watches commands and output in real time, and speaks rapid, assertive coaching through the OpenAI Realtime API.

End users are "computer illiterate" executives; the product ships as a single binary. Stack: Go + creack/pty + malgo + OpenAI Realtime, behind a provider-agnostic `Agent` interface.

## Status

PTY-backed shell wrapping, live voice coaching, and structured JSON logging are in place. Three selectable persona levels (beginner/intermediate/advanced) enforce a terse, default-silent coaching style. Next milestone is the "lab" framework for running scored experiments against a task corpus so persona iteration can be evidence-driven — see [TODO.md](TODO.md).

## Quick start

Requires Go 1.25+, `make`, and `OPENAI_API_KEY` in `.env`.

```bash
make build                                         # ./bin/coach
make run                                           # wrap your shell, live voice
./bin/coach --goal "find files > 100MB in ~"       # session goal shapes coaching
./bin/coach --persona advanced                     # less chatty for competent users
./bin/coach dev voices                             # audition voices one by one
make run-headless                                  # scripted scenario, no audio, for tuning
make tail                                          # pretty-tail logs/coach.log (uses jq if present)
make test
```

Exit the wrapped shell (`exit` or Ctrl-D) to shut coach down cleanly; Ctrl-C is passed through to the shell, not to coach.

## How it coaches

- Spawns a clean `/bin/bash --noprofile --norc` under a PTY with a stripped env. Your host shell config (starship, autosuggest, syntax-highlighting) does not load — the learner gets a predictable shell and the coach sees predictable output.
- Shell activity is sanitized (ANSI escapes, private-use / nerd-font glyphs, duplicate chunks) and streamed to the Realtime model as context.
- About 1.5s after shell activity settles, the model is asked to evaluate. It speaks only on errors, typos, dangerous commands, or drift from `--goal`; otherwise silent.
- After 15s of total idle, a nudge timer prompts the user to keep moving.
- The mic is muted while the coach is speaking, so the model doesn't react to its own voice coming back through the speakers.
- A session-level `max_response_output_tokens` cap plus an 8-word-per-turn persona rule keep responses terse ("Typo. Try history.", "Permission denied. Add sudo.").

## Config

| Flag | Default | What |
| --- | --- | --- |
| `--persona` | `beginner` | `beginner` (most protective) / `intermediate` (light-touch) / `advanced` (minimal) |
| `--voice` | `marin` | OpenAI Realtime voice; try `coach dev voices` to audition |
| `--goal "…"` | — | SESSION GOAL fed to the persona; drift triggers coaching |
| `--shell` | `/bin/bash --noprofile --norc` | Override the wrapped shell |
| `--scenario <name>` | — | Run a scripted scenario instead of wrapping the real shell (dev/tuning) |
| `--no-audio` | off | Logs only; skip mic and speaker |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` (env var) |

## Subcommands

- `coach dev voices [--only <name>] [--text "…"]` — sequentially play a sample phrase in each Realtime voice. Press Enter to advance, `q<Enter>` to quit.

## Docs

- [AGENTS.md](AGENTS.md) — stack, conventions, how agents should work in this repo.
- [TODO.md](TODO.md), [CHANGELOG.md](CHANGELOG.md), [LEARNINGS.md](LEARNINGS.md) — agent-maintained notebooks.
