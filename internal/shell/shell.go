package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type EventKind string

const (
	Input  EventKind = "input"
	Output EventKind = "output"
)

type Event struct {
	Kind EventKind
	Data []byte
}

// Session is a PTY-wrapped child shell. The user's real stdin/stdout are
// forwarded through transparently; every chunk is also mirrored to Events.
type Session struct {
	cmd      *exec.Cmd
	ptmx     *os.File
	events   chan Event
	oldState *term.State
	winchCh  chan os.Signal
	done     chan struct{}
}

// Start spawns a shell under a PTY, puts os.Stdin in raw mode, and pumps bytes
// in both directions. When shellPath is empty, defaults to a clean bash with
// no profile/rc and a minimal env — the user sees a predictable prompt and
// the coach sees predictable output (no starship, autosuggest, syntax
// highlighting, or other interactive plugins from the host's config).
func Start(ctx context.Context, shellPath string) (*Session, error) {
	var args []string
	if shellPath == "" {
		shellPath, args = defaultShell()
	} else {
		args = shellArgs(shellPath)
	}

	cmd := exec.CommandContext(ctx, shellPath, args...)
	cmd.Env = cleanEnv(shellPath)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty.Start: %w", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("term.MakeRaw: %w", err)
	}

	s := &Session{
		cmd:      cmd,
		ptmx:     ptmx,
		events:   make(chan Event, 128),
		oldState: oldState,
		winchCh:  make(chan os.Signal, 1),
		done:     make(chan struct{}),
	}

	signal.Notify(s.winchCh, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-s.done:
				return
			case <-s.winchCh:
				_ = pty.InheritSize(os.Stdin, ptmx)
			}
		}
	}()
	s.winchCh <- syscall.SIGWINCH

	// stdin -> pty, mirror to events
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				if _, werr := ptmx.Write(chunk); werr != nil {
					return
				}
				select {
				case s.events <- Event{Kind: Input, Data: chunk}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// pty -> stdout, mirror to events
	go func() {
		defer close(s.events)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				_, _ = os.Stdout.Write(chunk)
				select {
				case s.events <- Event{Kind: Output, Data: chunk}:
				default:
				}
			}
			if err != nil {
				if err != io.EOF {
					// surface ptmx read errors via done channel; caller sees via Wait
				}
				return
			}
		}
	}()

	return s, nil
}

func (s *Session) Events() <-chan Event { return s.events }

// Wait blocks until the child shell exits.
func (s *Session) Wait() error { return s.cmd.Wait() }

// defaultShell returns a shell path and startup args that skip user config.
// Bash is preferred for predictability and ubiquitous documentation; falls
// back to /bin/sh if bash isn't present.
func defaultShell() (string, []string) {
	for _, p := range []string{"/bin/bash", "/usr/bin/bash"} {
		if _, err := os.Stat(p); err == nil {
			return p, shellArgs(p)
		}
	}
	return "/bin/sh", nil
}

// shellArgs returns the flags needed to skip user config for a given shell.
func shellArgs(shellPath string) []string {
	base := shellPath
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	switch base {
	case "bash":
		return []string{"--noprofile", "--norc"}
	case "zsh":
		return []string{"-f"} // skip all startup files
	}
	return nil
}

// cleanEnv builds a minimal environment so the child shell doesn't pick up
// the host's prompt/plugin variables. Keeps enough to make the shell usable
// (PATH, HOME, TERM, locale) and sets a predictable PS1.
func cleanEnv(shellPath string) []string {
	keep := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
		"TERM": true, "TMPDIR": true, "SHLVL": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		if keep[kv[:i]] {
			env = append(env, kv)
		}
	}
	env = append(env,
		`PS1=\n\w $ `,
		"PS2=> ",
		"HISTFILE=/dev/null",
		"PROMPT_COMMAND=",
		"COACH=1",
		"BASH_SILENCE_DEPRECATION_WARNING=1", // macOS bash 3.2 startup banner
	)
	return env
}

// Close restores the terminal, stops signal handling, and closes the PTY.
// Safe to call multiple times.
func (s *Session) Close() error {
	select {
	case <-s.done:
		return nil
	default:
		close(s.done)
	}
	signal.Stop(s.winchCh)
	if s.oldState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), s.oldState)
		s.oldState = nil
	}
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
	return nil
}
