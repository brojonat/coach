package scenario

import (
	"context"
	"time"
)

type Event struct {
	At   time.Duration
	Text string
}

type Scenario struct {
	Name   string
	Goal   string
	Events []Event
}

var Baseline = Scenario{
	Name: "baseline",
	Goal: "The user wants to find what's eating disk space in their home directory and delete large files they don't need.",
	Events: []Event{
		{2 * time.Second, "user typed: ls"},
		{4 * time.Second, "shell printed: Desktop  Documents  Downloads  projects  Videos"},
		{7 * time.Second, "user typed: cd proejcts"},
		{8 * time.Second, "shell printed: bash: cd: proejcts: No such file or directory"},
		{14 * time.Second, "user typed: cd projects"},
		{16 * time.Second, "user typed: du -sh *"},
		{18 * time.Second, "command du -sh * is running..."},
		{28 * time.Second, "shell printed output: 2.1G  old-dataset  480M  coach  12K  notes.md"},
		{34 * time.Second, "user is idle"},
		{50 * time.Second, "user still idle, 16 seconds since last input"},
		{60 * time.Second, "user typed: rm -rf old-dataset"},
	},
}

var scenarios = map[string]Scenario{
	Baseline.Name: Baseline,
}

func Get(name string) (Scenario, bool) {
	s, ok := scenarios[name]
	return s, ok
}

func (s Scenario) Play(ctx context.Context) <-chan Event {
	out := make(chan Event)
	go func() {
		defer close(out)
		start := time.Now()
		for _, e := range s.Events {
			delay := e.At - time.Since(start)
			if delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
			select {
			case <-ctx.Done():
				return
			case out <- e:
			}
		}
	}()
	return out
}
