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

	// SendContext injects a text context item into the conversation. It does
	// NOT request a spoken response — call TriggerResponse for that. Use this
	// to keep the model aware of terminal activity without making it speak.
	SendContext(text string) error

	// TriggerResponse asks the model to generate a response now. Optional
	// instructions augment (not replace) the session persona for this turn —
	// use them to nudge the model toward a specific decision, e.g. "review
	// recent terminal output; stay silent unless the user is off-goal."
	TriggerResponse(instructions string) error

	// AudioOut streams 24kHz mono PCM16 LE audio chunks produced by the model.
	AudioOut() <-chan []byte

	// Transcript streams the model's spoken text deltas (what it is saying),
	// useful for logging and TUI display.
	Transcript() <-chan string

	Close() error
}
