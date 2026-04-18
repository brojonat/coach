package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"coach/internal/agent"
	"coach/internal/audio"
	"coach/internal/persona"
	"coach/internal/scenario"
	"coach/internal/shell"
)

type runOpts struct {
	Model    string
	Voice    string
	Persona  string
	Scenario string
	Goal     string
	Shell    string
	NoAudio  bool
}

func main() {
	initLogger()

	cmd := &cli.Command{
		Name:  "coach",
		Usage: "live terminal coaching agent",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "model", Value: "gpt-realtime", Usage: "realtime model id"},
			&cli.StringFlag{Name: "voice", Value: "marin", Usage: "voice name (marin, ash, cedar, coral, ...)"},
			&cli.StringFlag{Name: "persona", Value: "beginner", Usage: "persona: beginner | intermediate | advanced"},
			&cli.StringFlag{Name: "scenario", Value: "", Usage: "run a scripted scenario instead of wrapping the real shell"},
			&cli.StringFlag{Name: "goal", Value: "", Usage: "what the user is trying to accomplish; shapes what counts as off-track"},
			&cli.StringFlag{Name: "shell", Value: "", Usage: "shell to wrap (default: clean /bin/bash with no profile/rc)"},
			&cli.BoolFlag{Name: "no-audio", Usage: "headless mode, skip mic+speaker"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return run(ctx, runOpts{
				Model:    c.String("model"),
				Voice:    c.String("voice"),
				Persona:  c.String("persona"),
				Scenario: c.String("scenario"),
				Goal:     c.String("goal"),
				Shell:    c.String("shell"),
				NoAudio:  c.Bool("no-audio"),
			})
		},
		Commands: []*cli.Command{devCommand()},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		slog.Error("run failed", "source", "main", "err", err)
		os.Exit(1)
	}
}

// initLogger wires slog to emit JSON on stderr. LOG_LEVEL controls verbosity
// (debug|info|warn|error; default info).
func initLogger() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}

func run(ctx context.Context, opts runOpts) error {
	log := slog.With("source", "main")
	ptyMode := opts.Scenario == ""

	p, ok := persona.Get(opts.Persona)
	if !ok {
		return fmt.Errorf("unknown persona: %s", opts.Persona)
	}

	ag := agent.NewOpenAIRealtime(opts.Model)
	if err := ag.Connect(ctx, p.Instructions, opts.Voice); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer ag.Close()
	log.Info("connected", "model", opts.Model, "voice", opts.Voice, "persona", p.Name, "mode", map[bool]string{true: "pty", false: "scenario"}[ptyMode])

	if !opts.NoAudio {
		aio, err := audio.New()
		if err != nil {
			return fmt.Errorf("audio: %w", err)
		}
		defer aio.Close()
		if err := aio.Start(); err != nil {
			return fmt.Errorf("audio start: %w", err)
		}
		go pumpMic(ctx, aio, ag)
		go pumpSpeaker(ctx, aio, ag)
	} else {
		go func() {
			for range ag.AudioOut() {
			}
		}()
	}

	// Buffer transcript deltas; emit one log entry per completed coach turn.
	go func() {
		coachLog := slog.With("source", "coach")
		var buf strings.Builder
		for t := range ag.Transcript() {
			if t == "\n" {
				text := strings.TrimSpace(buf.String())
				buf.Reset()
				if text != "" {
					coachLog.Info("transcript", "text", text)
				}
				continue
			}
			buf.WriteString(t)
		}
	}()

	if opts.Goal != "" {
		if err := ag.SendContext("SESSION GOAL: " + opts.Goal); err != nil {
			log.Error("send goal", "err", err)
		}
	}

	if ptyMode {
		return runPTY(ctx, ag, opts.Goal, opts.Shell)
	}
	return runScenario(ctx, ag, opts.Scenario)
}

const brevityRules = "MAX 8 WORDS. Fragments only. No preamble, no hedging, no 'it looks like', 'it seems', 'you may', 'let me'. Imperative verbs first."

// reactInstructions nudges the model to stay quiet unless coaching is needed.
// Fired after shell activity settles. Augments the session persona.
func reactInstructions(goal string) string {
	base := brevityRules + " Review the most recent TERMINAL OUTPUT. Speak ONLY on: an error, a typo, a dangerous command, or drift from the session goal. Otherwise silent — no audio at all. Ignore shell startup banners."
	if goal != "" {
		base += " Goal: " + goal
	}
	return base
}

// nudgeInstructions is fired when the user has gone quiet. The model SHOULD
// speak here — briefly — to prompt the user's next move.
func nudgeInstructions(goal string) string {
	base := brevityRules + " User idle. Prompt them: one short question or next step."
	if goal != "" {
		base += " Goal: " + goal
	}
	return base
}

func runPTY(ctx context.Context, ag agent.Agent, goal, shellPath string) error {
	log := slog.With("source", "main")
	sh, err := shell.Start(ctx, shellPath)
	if err != nil {
		return fmt.Errorf("shell: %w", err)
	}
	defer sh.Close()

	const (
		reactDelay = 1500 * time.Millisecond
		nudgeDelay = 15 * time.Second
	)

	chunks := shell.Batch(ctx, sh.Events(), 300*time.Millisecond)
	react := time.NewTimer(time.Hour)
	react.Stop()
	nudge := time.NewTimer(nudgeDelay)
	reactInst := reactInstructions(goal)
	nudgeInst := nudgeInstructions(goal)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case text, ok := <-chunks:
				if !ok {
					return
				}
				if err := ag.SendContext(text); err != nil {
					log.Error("send context", "err", err)
					continue
				}
				react.Reset(reactDelay)
				nudge.Reset(nudgeDelay)
			case <-react.C:
				if err := ag.TriggerResponse(reactInst); err != nil {
					log.Error("react trigger", "err", err)
				}
				nudge.Reset(nudgeDelay)
			case <-nudge.C:
				if err := ag.TriggerResponse(nudgeInst); err != nil {
					log.Error("nudge trigger", "err", err)
				}
				nudge.Reset(nudgeDelay)
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- sh.Wait() }()

	select {
	case <-ctx.Done():
	case err := <-done:
		if err != nil {
			log.Info("shell exited", "err", err)
		}
	}
	log.Info("shutting down")
	return nil
}

func runScenario(ctx context.Context, ag agent.Agent, name string) error {
	log := slog.With("source", "main")
	s, ok := scenario.Get(name)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", name)
	}
	if s.Goal != "" {
		if err := ag.SendContext("SESSION GOAL: " + s.Goal); err != nil {
			log.Error("send goal", "err", err)
		}
	}
	go func() {
		log.Info("playing scenario", "name", s.Name, "events", len(s.Events))
		for ev := range s.Play(ctx) {
			log.Info("scenario event", "at", ev.At.String(), "text", ev.Text)
			if err := ag.SendContext("TERMINAL EVENT: " + ev.Text); err != nil {
				log.Error("send context", "err", err)
				continue
			}
			if err := ag.TriggerResponse(""); err != nil {
				log.Error("trigger", "err", err)
			}
		}
		log.Info("scenario complete")
	}()
	<-ctx.Done()
	log.Info("shutting down")
	return nil
}

func pumpMic(ctx context.Context, aio *audio.IO, ag agent.Agent) {
	log := slog.With("source", "main")
	for {
		select {
		case <-ctx.Done():
			return
		case buf, ok := <-aio.InCh:
			if !ok {
				return
			}
			if err := ag.SendUserAudio(buf); err != nil {
				log.Error("send audio", "err", err)
				return
			}
		}
	}
}

func pumpSpeaker(ctx context.Context, aio *audio.IO, ag agent.Agent) {
	for {
		select {
		case <-ctx.Done():
			return
		case buf, ok := <-ag.AudioOut():
			if !ok {
				return
			}
			aio.Play(buf)
		}
	}
}
