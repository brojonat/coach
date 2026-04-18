package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"coach/internal/agent"
	"coach/internal/audio"
)

// availableVoices is the current set of gpt-realtime voices. Edit as OpenAI
// adds/removes them.
var availableVoices = []string{
	"alloy", "ash", "ballad", "cedar", "coral",
	"echo", "marin", "sage", "shimmer", "verse",
}

func devCommand() *cli.Command {
	return &cli.Command{
		Name:  "dev",
		Usage: "developer utilities",
		Commands: []*cli.Command{
			{
				Name:  "voices",
				Usage: "play a sample line in each available voice",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "model", Value: "gpt-realtime", Usage: "realtime model id"},
					&cli.StringFlag{Name: "text", Value: "Hi, I'm your terminal coach. Let's get started.", Usage: "sample phrase to speak"},
					&cli.StringSliceFlag{Name: "only", Usage: "limit to specific voice names (repeatable)"},
				},
				Action: voicesAction,
			},
		},
	}
}

func voicesAction(ctx context.Context, c *cli.Command) error {
	sample := c.String("text")
	model := c.String("model")
	voices := c.StringSlice("only")
	if len(voices) == 0 {
		voices = availableVoices
	}

	aio, err := audio.New()
	if err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	defer aio.Close()
	if err := aio.Start(); err != nil {
		return fmt.Errorf("audio start: %w", err)
	}

	const instructions = "You are a voice sample. Say exactly the text you are asked to say, then stop. No preamble, no interpretation."

	reader := bufio.NewReader(os.Stdin)
	for _, v := range voices {
		fmt.Fprintf(os.Stdout, "\n--- voice: %s ---\n", v)
		if err := playSample(ctx, model, v, instructions, sample, aio); err != nil {
			fmt.Fprintf(os.Stdout, "  error: %v\n", err)
		}
		if ctx.Err() != nil {
			return nil
		}
		fmt.Fprint(os.Stdout, "[Enter] next | [q Enter] quit: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "q") {
			return nil
		}
	}
	return nil
}

// playSample connects a fresh session with the given voice, plays one sample
// utterance, and tears down. Blocks until the audio transcript reports done
// (or a timeout fires).
func playSample(ctx context.Context, model, voice, instructions, sample string, aio *audio.IO) error {
	ag := agent.NewOpenAIRealtime(model)
	if err := ag.Connect(ctx, instructions, voice); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer ag.Close()

	done := make(chan struct{}, 1)
	go func() {
		for t := range ag.Transcript() {
			if t == "\n" {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		}
	}()
	go func() {
		for buf := range ag.AudioOut() {
			aio.Play(buf)
		}
	}()

	if err := ag.TriggerResponse("Say exactly: " + sample); err != nil {
		return fmt.Errorf("trigger: %w", err)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	// Let queued audio drain from the output device before the session closes.
	time.Sleep(600 * time.Millisecond)
	return nil
}
