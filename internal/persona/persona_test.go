package persona

import (
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"known persona", "assertive-coach", true},
		{"unknown persona", "does-not-exist", false},
		{"empty key", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := Get(tc.key)
			if ok != tc.want {
				t.Errorf("Get(%q) ok = %v, want %v", tc.key, ok, tc.want)
			}
		})
	}
}

func TestAssertiveCoach(t *testing.T) {
	p, ok := Get("assertive-coach")
	if !ok {
		t.Fatal("assertive-coach missing")
	}
	if p.Name != "assertive-coach" {
		t.Errorf("Name = %q, want %q", p.Name, "assertive-coach")
	}
	if len(p.Instructions) < 200 {
		t.Errorf("Instructions too short: %d chars", len(p.Instructions))
	}
	// Spot-check that core persona anchors survived any edit.
	for _, want := range []string{"assertive", "STOP", "GOAL"} {
		if !strings.Contains(p.Instructions, want) {
			t.Errorf("Instructions missing anchor %q", want)
		}
	}
}
