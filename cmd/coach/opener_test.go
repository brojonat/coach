package main

import (
	"strings"
	"testing"
)

func TestOpeningInstructionsWithGoal(t *testing.T) {
	goal := "find files over 100MB in home"
	got := openingInstructions(goal)

	if !strings.Contains(got, "Goal: "+goal) {
		t.Fatalf("expected goal to be embedded, got: %q", got)
	}
	// Must include the silence-override so the model actually speaks.
	if !strings.Contains(got, "do NOT stay silent") {
		t.Fatalf("expected explicit silence override, got: %q", got)
	}
	// Must still carry the brevity rules so the opener stays terse.
	if !strings.Contains(got, "MAX 8 WORDS") {
		t.Fatalf("expected brevity rules, got: %q", got)
	}
}

func TestOpeningInstructionsNoGoal(t *testing.T) {
	got := openingInstructions("")

	if strings.Contains(got, "Goal:") {
		t.Fatalf("no-goal branch should not embed a Goal: line, got: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "what") {
		t.Fatalf("no-goal branch should ask what the user wants, got: %q", got)
	}
	if !strings.Contains(got, "do NOT stay silent") {
		t.Fatalf("expected explicit silence override, got: %q", got)
	}
}
