package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const defaultLogPath = "logs/coach.log"

type watchOpts struct {
	Path      string
	All       bool
	FromStart bool
	NoColor   bool
}

func watchCommand() *cli.Command {
	return &cli.Command{
		Name:  "watch",
		Usage: "follow a coach log in another pane/window (pretty-print coach turns + errors)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "log", Value: defaultLogPath, Usage: "log file to follow"},
			&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "show every event, not just coach turns + errors"},
			&cli.BoolFlag{Name: "from-start", Usage: "read the whole file before following (default: follow from EOF)"},
			&cli.BoolFlag{Name: "no-color", Usage: "disable ANSI color output"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return runWatch(ctx, watchOpts{
				Path:      c.String("log"),
				All:       c.Bool("all"),
				FromStart: c.Bool("from-start"),
				NoColor:   c.Bool("no-color"),
			})
		},
	}
}

func runWatch(ctx context.Context, opts watchOpts) error {
	color := !opts.NoColor && term.IsTerminal(int(os.Stdout.Fd()))
	r := renderer{color: color, all: opts.All}

	if err := waitForFile(ctx, opts.Path); err != nil {
		return err
	}

	f, err := os.Open(opts.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	if !opts.FromStart {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "coach watch — following %s (Ctrl-C to stop)\n", opts.Path)

	br := bufio.NewReader(f)
	var partial strings.Builder
	for {
		if ctx.Err() != nil {
			return nil
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			partial.WriteString(line)
			if strings.HasSuffix(line, "\n") {
				if out := r.render(partial.String()); out != "" {
					fmt.Print(out)
				}
				partial.Reset()
			}
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			return err
		}
		// EOF: handle truncation/rotation, then poll for new data.
		if rotated, rerr := handleRotation(opts.Path, &f, &br); rerr != nil {
			return rerr
		} else if rotated {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// waitForFile blocks until path exists or ctx is cancelled.
func waitForFile(ctx context.Context, path string) error {
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %q: %w", path, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// handleRotation detects truncation (size shrank below our read position) and
// reopens the file from the start. Returns true if we reopened.
func handleRotation(path string, f **os.File, br **bufio.Reader) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		// File disappeared — treat as non-fatal; caller will poll again.
		return false, nil
	}
	pos, err := (*f).Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	if info.Size() >= pos {
		return false, nil
	}
	nf, err := os.Open(path)
	if err != nil {
		return false, err
	}
	(*f).Close()
	*f = nf
	*br = bufio.NewReader(nf)
	return true, nil
}

// renderer converts a JSON log line to a human-friendly line (or "" to skip).
type renderer struct {
	color bool
	all   bool
}

func (r renderer) render(line string) string {
	line = strings.TrimRight(line, "\n")
	if line == "" {
		return ""
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		// Not JSON — pass through verbatim.
		return line + "\n"
	}

	level, _ := rec["level"].(string)
	source, _ := rec["source"].(string)
	msg, _ := rec["msg"].(string)
	text, _ := rec["text"].(string)
	stamp := parseTime(rec["time"]).Format("15:04:05")

	switch {
	case source == "coach" && msg == "transcript" && text != "":
		return fmt.Sprintf("%s %s %s\n",
			r.paint(stamp, colorDim),
			r.paint("coach>", colorCyanBold),
			text,
		)
	case level == "ERROR":
		return fmt.Sprintf("%s %s %-6s %s%s\n",
			r.paint(stamp, colorDim),
			r.paint("ERROR", colorRed),
			source,
			msg,
			extraAttrs(rec),
		)
	case level == "WARN":
		return fmt.Sprintf("%s %s %-6s %s%s\n",
			r.paint(stamp, colorDim),
			r.paint("WARN ", colorYellow),
			source,
			msg,
			extraAttrs(rec),
		)
	default:
		if !r.all {
			return ""
		}
		return fmt.Sprintf("%s %-5s %-6s %s%s\n",
			r.paint(stamp, colorDim),
			strings.ToLower(level),
			source,
			msg,
			extraAttrs(rec),
		)
	}
}

const (
	colorReset    = "\033[0m"
	colorDim      = "\033[2m"
	colorRed      = "\033[31m"
	colorYellow   = "\033[33m"
	colorCyanBold = "\033[1;36m"
)

func (r renderer) paint(s, code string) string {
	if !r.color {
		return s
	}
	return code + s + colorReset
}

// standardKeys are the slog fields we render explicitly; everything else is
// appended as key=value to the tail of the line.
var standardKeys = map[string]bool{
	"time":   true,
	"level":  true,
	"source": true,
	"msg":    true,
	"text":   true,
}

func extraAttrs(rec map[string]any) string {
	keys := make([]string, 0, len(rec))
	for k := range rec {
		if !standardKeys[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, rec[k]))
	}
	return " " + strings.Join(parts, " ")
}

func parseTime(v any) time.Time {
	s, _ := v.(string)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
