package main

import (
	"strings"
	"testing"
)

func TestRenderCoachTranscript(t *testing.T) {
	r := renderer{color: false, all: false}
	in := `{"time":"2026-04-18T04:36:50.013567-07:00","level":"INFO","source":"coach","msg":"transcript","text":"Typo. Try history."}` + "\n"
	out := r.render(in)
	if out == "" {
		t.Fatal("expected coach transcript to render, got empty")
	}
	if !strings.Contains(out, "coach>") || !strings.Contains(out, "Typo. Try history.") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "04:36:50") {
		t.Fatalf("expected HH:MM:SS stamp in output, got %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("expected trailing newline, got %q", out)
	}
}

func TestRenderErrorAlwaysShown(t *testing.T) {
	r := renderer{color: false, all: false}
	in := `{"time":"2026-04-18T04:36:50Z","level":"ERROR","source":"main","msg":"connect failed","err":"dial tcp: refused"}` + "\n"
	out := r.render(in)
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "connect failed") {
		t.Fatalf("error line not rendered properly: %q", out)
	}
	if !strings.Contains(out, "err=dial tcp: refused") {
		t.Fatalf("expected extra attrs appended: %q", out)
	}
}

func TestRenderWarnShown(t *testing.T) {
	r := renderer{color: false, all: false}
	in := `{"time":"2026-04-18T04:36:50Z","level":"WARN","source":"agent","msg":"backoff"}` + "\n"
	out := r.render(in)
	if !strings.Contains(out, "WARN") || !strings.Contains(out, "backoff") {
		t.Fatalf("warn line not rendered: %q", out)
	}
}

func TestRenderDefaultSkipsInfoNoise(t *testing.T) {
	r := renderer{color: false, all: false}
	in := `{"time":"2026-04-18T04:36:50Z","level":"INFO","source":"main","msg":"connected","model":"gpt-realtime"}` + "\n"
	if out := r.render(in); out != "" {
		t.Fatalf("expected default mode to skip info noise, got %q", out)
	}
}

func TestRenderAllShowsEverything(t *testing.T) {
	r := renderer{color: false, all: true}
	in := `{"time":"2026-04-18T04:36:50Z","level":"INFO","source":"main","msg":"connected","model":"gpt-realtime"}` + "\n"
	out := r.render(in)
	if !strings.Contains(out, "connected") || !strings.Contains(out, "model=gpt-realtime") {
		t.Fatalf("--all should render everything, got %q", out)
	}
}

func TestRenderNonJSONPassthrough(t *testing.T) {
	r := renderer{color: false, all: false}
	in := "not json at all\n"
	out := r.render(in)
	if out != "not json at all\n" {
		t.Fatalf("expected passthrough, got %q", out)
	}
}

func TestRenderEmptyLine(t *testing.T) {
	r := renderer{color: false, all: true}
	if out := r.render("\n"); out != "" {
		t.Fatalf("empty line should produce no output, got %q", out)
	}
}

func TestRenderColorEnabled(t *testing.T) {
	r := renderer{color: true, all: false}
	in := `{"time":"2026-04-18T04:36:50Z","level":"INFO","source":"coach","msg":"transcript","text":"Stop."}` + "\n"
	out := r.render(in)
	if !strings.Contains(out, colorCyanBold) || !strings.Contains(out, colorReset) {
		t.Fatalf("expected ANSI color codes in output, got %q", out)
	}
}

func TestExtraAttrsSortedAndStandardElided(t *testing.T) {
	rec := map[string]any{
		"time":   "x",
		"level":  "INFO",
		"source": "main",
		"msg":    "hi",
		"text":   "ignored",
		"zebra":  1,
		"apple":  "fruit",
	}
	got := extraAttrs(rec)
	// zebra + apple only, sorted alphabetically.
	if got != " apple=fruit zebra=1" {
		t.Fatalf("unexpected extras: %q", got)
	}
}
