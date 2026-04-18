# AGENTS.md

Conventions for AI agents (and humans) working on `coach`.

## Project

`coach` is a live terminal coaching agent for non-technical users. The product watches a user's shell in real time and provides commanding, high-energy voice guidance (think Dota 2 live coaching). Target end users are executives who are "computer illiterate" in the terminal sense — the product must ship as a single binary with no runtime dependencies.

## Stack

- **Language:** Go 1.25+
- **CLI framework:** `github.com/urfave/cli/v3` (not stdlib `flag`)
- **Voice/realtime:** `github.com/coder/websocket` + OpenAI Realtime API (first impl behind `internal/agent.Agent` interface; provider-agnostic)
- **Audio:** `github.com/gen2brain/malgo` (miniaudio, self-contained, cross-platform)
- **TUI (later):** Bubble Tea + Lipgloss
- **Database:** when needed, use `sqlc` for all queries. No raw SQL strings outside sqlc-generated code. No ORMs.

## Layout

```
cmd/coach/          # main entry
internal/agent/     # voice-agent interface + provider impls
internal/audio/     # mic/speaker I/O
internal/persona/   # persona prompts
internal/scenario/  # scripted terminal-event streams (for persona tuning)
```

## Conventions

### Entry points → Makefile

All main entry points go through the Makefile. Don't document raw `go build` / `go run` invocations in READMEs; document `make run`, `make test`, etc.

Targets load environment from `.env` using:

```make
@set -a; . ./.env; set +a; <command>
```

`.env` is gitignored. Never commit it.

### Testing — red/green TDD

New behavior is written test-first:

1. Write a failing test that expresses the desired behavior.
2. Confirm it fails (`make test`) for the right reason.
3. Implement the minimum code to make it pass.
4. Refactor with the green test as a safety net.

Run `make test` before every commit. Prefer table-driven tests. Keep tests fast — no network or real audio devices in unit tests; use the `Agent` interface for fakes.

### Provider-agnostic voice layer

The `Agent` interface in `internal/agent/agent.go` is the boundary. No OpenAI-specific types may leak into `cmd/` or other packages. Adding a new provider (Gemini Live, Anthropic + ElevenLabs) is a new file implementing `Agent`, not a refactor.

### Persona

The coaching persona is **assertive, loud, commanding** — not polite. That energy is the feature. Mute is the user's escape valve, not a consent gate.

## Agent-maintained files

Three markdown files at the repo root are lightweight notebooks for agents. **Agents should update these at the end of a task**, not during — treat them as wrap-up artifacts, part of "done."

- **`TODO.md`** — running list of things to do. Agents may add, reorder, and check off items. Use checkboxes (`- [ ]`). Remove an item once it's captured elsewhere (CHANGELOG, commit, closed issue).
- **`CHANGELOG.md`** — **append-only** log of notable changes. One dated entry per task/PR. Never rewrite prior entries. Brevity beats ceremony.
- **`LEARNINGS.md`** — things future agents should remember to avoid or do differently. Failed approaches, surprising gotchas, API footguns. Append only; never delete a learning just because it's inconvenient.

## Common commands

```bash
make build         # build binary to ./bin/coach
make run           # full voice loop (mic + speaker + scenario)
make run-headless  # no audio, print transcripts + raw events
make test          # go test ./...
make vet           # go vet ./...
make tidy          # go mod tidy
make clean         # rm -rf bin/
```

## Secrets

`.env` holds `OPENAI_API_KEY` and other credentials. Gitignored. Never echo, log, or commit.
