# coach

Live terminal coaching agent for non-technical users. `coach` watches a user's shell in real time and provides commanding, high-energy voice guidance (think Dota 2 live coaching) via the OpenAI Realtime API.

End users are "computer illiterate" executives; the product must ship as a single binary. Stack is Go + Bubble Tea + malgo + OpenAI Realtime, behind a provider-agnostic `Agent` interface.

## Status

Early scaffold. Persona tuning against scripted terminal-event scenarios; real PTY wiring is next.

## Quick start

Requires Go 1.25+, `make`, and `OPENAI_API_KEY` in `.env`.

```bash
make build            # ./bin/coach
make run              # full voice loop (mic + speaker + scripted scenario)
make run-headless     # no audio, print transcripts + raw events to logs/headless.log
make test             # unit tests
```

## Docs

- [AGENTS.md](AGENTS.md) — stack, conventions, and how agents should work in this repo.
- [TODO.md](TODO.md), [CHANGELOG.md](CHANGELOG.md), [LEARNINGS.md](LEARNINGS.md) — agent-maintained notebooks; see AGENTS.md.
