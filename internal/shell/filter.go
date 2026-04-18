package shell

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// ansiRE strips escape sequences and control noise so the LLM sees readable text.
// Order matters: more specific multi-byte forms first, then the catch-alls.
var ansiRE = regexp.MustCompile(
	`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]` + // CSI ESC[ ... final (full range)
		`|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC ESC] ... BEL or ST
		`|\x1b[PX^_][^\x1b]*\x1b\\` + // DCS/SOS/PM/APC ... ST
		`|\x1bO[\x40-\x7e]` + // SS3 ESC O x (function keys)
		`|\x1b[()*+][A-Za-z0-9]` + // charset select
		`|\x1b[78DEMHZc=>]` + // various single-char ESC sequences
		`|[\x00-\x08\x0b-\x0c\x0e-\x1f\x7f]`) // control chars (keep \n \t \r)

// glyphRE strips common non-text decoration used by Starship / powerline / nerd fonts.
// Private-use area is where nerd fonts live; we also drop the box-drawing and
// geometric shapes blocks that show up as prompt separators (❯ ▶ etc.).
var glyphRE = regexp.MustCompile(
	`[\x{E000}-\x{F8FF}` + // Private Use Area (nerd fonts)
		`\x{F0000}-\x{FFFFD}` + // Supplementary Private Use-A
		`\x{100000}-\x{10FFFD}` + // Supplementary Private Use-B
		`\x{2500}-\x{257F}` + // Box Drawing
		`\x{2580}-\x{259F}` + // Block Elements
		`\x{25A0}-\x{25FF}` + // Geometric Shapes
		`\x{2700}-\x{27BF}]`) // Dingbats (includes ❯ ❮)

var wsRE = regexp.MustCompile(`[ \t]+`)

func sanitize(s string) string {
	s = ansiRE.ReplaceAllString(s, "")
	s = glyphRE.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = wsRE.ReplaceAllString(s, " ")
	// Collapse runs of blank lines.
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// Batch consumes raw shell output events, sanitizes, debounces on idle,
// drops empty or duplicate chunks, and emits labeled text ready to be sent
// to the agent as context. Input (keystroke) events are ignored — the shell
// echoes typed chars into its own output, so we'd double-report.
func Batch(ctx context.Context, in <-chan Event, idle time.Duration) <-chan string {
	out := make(chan string, 32)
	go func() {
		defer close(out)
		var buf strings.Builder
		var lastSent string
		timer := time.NewTimer(time.Hour)
		timer.Stop()

		flush := func() {
			if buf.Len() == 0 {
				return
			}
			text := sanitize(buf.String())
			buf.Reset()
			if text == "" || text == lastSent {
				return
			}
			lastSent = text
			select {
			case out <- "TERMINAL OUTPUT:\n" + text:
			case <-ctx.Done():
			}
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case ev, ok := <-in:
				if !ok {
					flush()
					return
				}
				if ev.Kind != Output {
					continue
				}
				buf.Write(ev.Data)
				timer.Reset(idle)
			case <-timer.C:
				flush()
			}
		}
	}()
	return out
}
