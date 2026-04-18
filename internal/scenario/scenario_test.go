package scenario

import (
	"context"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	if _, ok := Get("baseline"); !ok {
		t.Error("baseline scenario missing")
	}
	if _, ok := Get("does-not-exist"); ok {
		t.Error("nonexistent scenario should return false")
	}
}

func TestPlayEmitsEventsInOrder(t *testing.T) {
	s := Scenario{
		Name: "test",
		Events: []Event{
			{10 * time.Millisecond, "a"},
			{20 * time.Millisecond, "b"},
			{30 * time.Millisecond, "c"},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var got []string
	for ev := range s.Play(ctx) {
		got = append(got, ev.Text)
	}

	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPlayStopsOnContextCancel(t *testing.T) {
	s := Scenario{
		Name: "test",
		Events: []Event{
			{10 * time.Millisecond, "a"},
			{500 * time.Millisecond, "b"}, // beyond ctx deadline
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var got []string
	for ev := range s.Play(ctx) {
		got = append(got, ev.Text)
	}

	if len(got) != 1 || got[0] != "a" {
		t.Errorf("got %v, want [a] only (ctx should have cancelled second event)", got)
	}
}

func TestPlayRespectsTiming(t *testing.T) {
	s := Scenario{
		Name: "test",
		Events: []Event{
			{50 * time.Millisecond, "a"},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	for range s.Play(ctx) {
	}
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Errorf("Play returned too fast: %v (expected >= 40ms)", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Play returned too slow: %v (expected <= 200ms)", elapsed)
	}
}
