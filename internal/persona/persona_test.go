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
		{"beginner", "beginner", true},
		{"intermediate", "intermediate", true},
		{"advanced", "advanced", true},
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

func TestAllPersonas(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			p, ok := Get(name)
			if !ok {
				t.Fatalf("persona %q not found via Get()", name)
			}
			if p.Name != name {
				t.Errorf("Name = %q, want %q", p.Name, name)
			}
			if len(p.Instructions) < 200 {
				t.Errorf("Instructions too short: %d chars", len(p.Instructions))
			}
			// Every persona must inherit the shared style rules and describe
			// the input format.
			for _, anchor := range []string{"DEFAULT MODE IS SILENCE", "TERMINAL OUTPUT", "MAX 8 WORDS"} {
				if !strings.Contains(p.Instructions, anchor) {
					t.Errorf("missing anchor %q", anchor)
				}
			}
		})
	}
}
