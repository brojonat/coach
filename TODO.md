# TODO

## Lab framework — prompt-optimization loop

Scored experiments against a task corpus so we can iterate on the persona with evidence instead of vibes.

- [ ] SQLite store at `coach.db` (pure-Go driver: `modernc.org/sqlite`); migrations run on open
- [ ] Schema: `tasks`, `experiments` (snapshots persona text + full config), `ratings`, `events`
- [ ] `coach lab tasks seed` — inject ~15 starter tasks spanning beginner/intermediate/advanced, tagged (navigation, git, files, network, dangerous)
- [ ] `coach lab tasks list [--tag X]`
- [ ] `coach lab tasks show <slug>`
- [ ] `coach lab tasks add` — interactive
- [ ] `coach lab run <task-slug>` — wraps `run`, snapshots persona text + config, records an experiment row, returns the id
- [ ] Custom `slog.Handler` mirrors every JSON entry into `events` during an active experiment (so the run is replayable)
- [ ] `coach lab rate [<experiment-id>]` — interactive 1-5 on overall / correctness / brevity / timing / intrusiveness, plus a free-text note
- [ ] `coach lab report` — aggregate by persona, task tag, rating dimension
- [ ] `coach lab report <experiment-id>` — single-run dump with transcript replay
- [ ] `coach lab tasks generate --count N` — LLM-brainstormed corpus (later, once manual rating reveals what a good task looks like)
- [ ] `coach lab run --agent claude` — a coding agent drives the task instead of a human (long-term vision)

## Coach follow-ups

- [ ] Prompt-redraw dedupe misses starship timestamp changes — smarter dedupe, or widen the nudge/react windows
- [ ] Investigate OSC 133 prompt markers for reliable command/output scoping (alternative to full VT emulation if we ever re-enable user-side shell config)
- [ ] "Lessons" concept — structured skill tutorials the coach can walk the user through (e.g. "learn git basics", "install a coding agent"); each lesson is a bundled task sequence + persona tweaks
