package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"coach/internal/agent"
	"coach/internal/audio"
	"coach/internal/persona"
	"coach/internal/scenario"
)

type runOpts struct {
	Model    string
	Voice    string
	Persona  string
	Scenario string
	NoAudio  bool
	Debug    bool
}

func main() {
	cmd := &cli.Command{
		Name:  "coach",
		Usage: "live terminal coaching agent",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "model", Value: "gpt-realtime", Usage: "realtime model id"},
			&cli.StringFlag{Name: "voice", Value: "marin", Usage: "voice name (marin, ash, cedar, coral, ...)"},
			&cli.StringFlag{Name: "persona", Value: "assertive-coach", Usage: "persona name"},
			&cli.StringFlag{Name: "scenario", Value: "baseline", Usage: "scenario name"},
			&cli.BoolFlag{Name: "no-audio", Usage: "headless mode, skip mic+speaker"},
			&cli.BoolFlag{Name: "debug", Usage: "log raw realtime server events"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return run(ctx, runOpts{
				Model:    c.String("model"),
				Voice:    c.String("voice"),
				Persona:  c.String("persona"),
				Scenario: c.String("scenario"),
				NoAudio:  c.Bool("no-audio"),
				Debug:    c.Bool("debug"),
			})
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, opts runOpts) error {
	p, ok := persona.Get(opts.Persona)
	if !ok {
		return fmt.Errorf("unknown persona: %s", opts.Persona)
	}
	s, ok := scenario.Get(opts.Scenario)
	if !ok {
		return fmt.Errorf("unknown scenario: %s", opts.Scenario)
	}

	ag := agent.NewOpenAIRealtime(opts.Model, opts.Debug)
	if err := ag.Connect(ctx, p.Instructions, opts.Voice); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer ag.Close()
	log.Printf("connected: model=%s voice=%s persona=%s scenario=%s", opts.Model, opts.Voice, p.Name, s.Name)

	if s.Goal != "" {
		if err := ag.SendContext("SESSION GOAL: " + s.Goal); err != nil {
			log.Printf("send goal: %v", err)
		}
	}

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

	go func() {
		for t := range ag.Transcript() {
			fmt.Print(t)
		}
	}()

	go func() {
		log.Printf("playing scenario: %s (%d events)", s.Name, len(s.Events))
		for ev := range s.Play(ctx) {
			log.Printf("event @%s: %s", ev.At, ev.Text)
			if err := ag.SendContext("TERMINAL EVENT: " + ev.Text); err != nil {
				log.Printf("send ctx: %v", err)
			}
		}
		log.Println("scenario complete; waiting on signal to exit")
	}()

	<-ctx.Done()
	log.Println("shutting down")
	return nil
}

func pumpMic(ctx context.Context, aio *audio.IO, ag agent.Agent) {
	for {
		select {
		case <-ctx.Done():
			return
		case buf, ok := <-aio.InCh:
			if !ok {
				return
			}
			if err := ag.SendUserAudio(buf); err != nil {
				log.Printf("send audio: %v", err)
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
