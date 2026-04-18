package agent

import "context"

// Agent is the provider-agnostic interface for a realtime voice coaching agent.
// Implementations wrap a specific vendor (OpenAI Realtime, Gemini Live, etc.).
type Agent interface {
	// Connect opens the realtime session. Instructions is the persona prompt;
	// voice is an implementation-specific voice id.
	Connect(ctx context.Context, instructions, voice string) error

	// SendUserAudio sends a chunk of user mic audio. Format is 24kHz mono PCM16 LE.
	SendUserAudio(chunk []byte) error

	// SendContext injects a text context item into the conversation — used for
	// scripted terminal events, goals, etc. The agent should react to it.
	SendContext(text string) error

	// AudioOut streams 24kHz mono PCM16 LE audio chunks produced by the model.
	AudioOut() <-chan []byte

	// Transcript streams the model's spoken text deltas (what it is saying),
	// useful for logging and TUI display.
	Transcript() <-chan string

	Close() error
}
