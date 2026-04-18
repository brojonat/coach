# Learnings

## OpenAI Realtime API

- **`SendContext` does not auto-trigger a response.** `conversation.item.create` silently adds the item; `response.create` is a separate required call. If you're expecting the model to react and nothing happens, check that a `response.create` went out.
- **Per-response `instructions` augment, they don't replace the session persona.** Style rules (brevity, banned phrases) must appear in BOTH the session persona AND the per-response instructions. Models weight the per-turn nudge heavily.
- **`max_response_output_tokens` is valid at the session level, NOT on `response.create`.** With `gpt-realtime`, sending `response.max_response_output_tokens` comes back as `invalid_request_error: unknown_parameter` — and since the trigger call fails, the model never speaks. Put the cap in `session.update` once.
- **The token cap mixes audio and text tokens.** Audio is ~50 tokens/sec, so a 40-token cap ≈ 1s of speech, which truncates responses mid-phrase ("Wrong command. Use:"). 400 is a reasonable safety belt; primary brevity control belongs in the prompt.
- **Server VAD + whisper creates a feedback loop.** Coach voice → speakers → mic → whisper → "user said" in the conversation → coach reacts to its own hallucinated input. Mute the mic while speaking (on `response.created`, unmute ~800ms after `response.done` so the speakers can drain).

## PTY / shell capture

- **ANSI stripping alone is not enough with zsh-autosuggestions / syntax-highlighting / fzf-tab.** Those plugins draw text mid-line, then use cursor-move escapes to overwrite. Stripping the escapes leaves the characters behind as a soup ("hhhistory", "bbbabar") that the model misreads as typos. Fix at the source: wrap a clean `/bin/bash --noprofile --norc` with a minimal env instead of inheriting the host's interactive setup.
- **Piping stdout through `tee` destroys the TTY.** With `make run | tee logs/run.log`, `isatty(stdout)` becomes false and PTY forwarding / shell colors / resize detection all break. Keep stdout attached to the real terminal; redirect stderr to a file instead (`$(BIN) 2>logs/coach.log`).
- **Two log sinks is a footgun.** We briefly had both `log.SetOutput` (Go-side) and `2>` (Makefile-side) pointing at different files; only one ever had content, the other was empty and confused me about "missing logs." Pick one sink, document it, stick with it.

## SQLite / lab store

- **`modernc.org/sqlite` registers as driver name `"sqlite"`, not `"sqlite3"`.** Using `sql.Open("sqlite3", ...)` with the pure-Go driver fails with `unknown driver`. Only `mattn/go-sqlite3` (cgo) uses `"sqlite3"`.
- **Keep `MaxOpenConns = 1` for write-heavy SQLite.** Concurrent writes on a single file serialize anyway; capping the pool makes contention surface as backpressure instead of `SQLITE_BUSY` retries.
- **`INSERT OR IGNORE` is the cheap idempotent seed.** Paired with a `UNIQUE` column (slug), it lets `seed` run repeatedly without extra round-trips or conflict handling. `RowsAffected()` tells you new-vs-skipped.

## Persona / prompt engineering

- **Listing bad examples can backfire.** "DO NOT say 'it looks like…'" with an example sometimes causes the model to mimic the example instead of avoiding it. A banned-opener list without any example sentences that use the banned forms works better.
- **Silence has to be trained twice.** The persona says "default mode is silence," but the model still speaks on every `response.create`. To actually stay quiet, don't trigger a response at all on uninteresting events — use a client-side debounce that only fires `response.create` when there's signal (activity quieting, goal drift, idle timeout).
