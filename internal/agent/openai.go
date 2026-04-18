package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const realtimeBaseURL = "wss://api.openai.com/v1/realtime"

// speakerDrain gives audio already buffered to the speakers time to finish
// playing before we unmute the mic. Keeps the coach from hearing its own tail.
const speakerDrain = 800 * time.Millisecond

type OpenAIRealtime struct {
	model      string
	conn       *websocket.Conn
	writeMu    sync.Mutex
	audioOut   chan []byte
	transcript chan string
	cancel     context.CancelFunc
	speaking   atomic.Bool
}

func NewOpenAIRealtime(model string) *OpenAIRealtime {
	return &OpenAIRealtime{
		model:      model,
		audioOut:   make(chan []byte, 64),
		transcript: make(chan string, 64),
	}
}

func (o *OpenAIRealtime) log() *slog.Logger {
	return slog.With("source", "agent")
}

func (o *OpenAIRealtime) Connect(ctx context.Context, instructions, voice string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY not set")
	}

	url := realtimeBaseURL + "?model=" + o.model
	header := http.Header{}
	header.Set("Authorization", "Bearer "+apiKey)
	header.Set("OpenAI-Beta", "realtime=v1")

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return fmt.Errorf("realtime dial: %w", err)
	}
	conn.SetReadLimit(1 << 22) // 4MB per frame; audio deltas can be large
	o.conn = conn

	runCtx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel

	o.log().Info("session init", "persona_chars", len(instructions), "persona", instructions)

	sess := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":          []string{"audio", "text"},
			"instructions":        instructions,
			"voice":               voice,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"turn_detection": map[string]any{
				"type":                "server_vad",
				"threshold":           0.5,
				"prefix_padding_ms":   300,
				"silence_duration_ms": 500,
			},
			"input_audio_transcription": map[string]any{"model": "whisper-1"},
			// Hard cap on per-response tokens — a safety belt, not the
			// primary brevity control (that's in the persona + react
			// instructions). The count mixes audio and text tokens, and
			// audio is dense: ~50 tokens per second of speech. 400 leaves
			// room for ~8 seconds, enough for two short clauses without
			// letting the model drone on.
			"max_response_output_tokens": 400,
		},
	}
	if err := o.send(ctx, sess); err != nil {
		return fmt.Errorf("session.update: %w", err)
	}

	go o.readLoop(runCtx)
	return nil
}

func (o *OpenAIRealtime) send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	o.writeMu.Lock()
	defer o.writeMu.Unlock()
	return o.conn.Write(ctx, websocket.MessageText, b)
}

// SendUserAudio forwards mic audio, but drops chunks while the coach is
// speaking so the coach doesn't hear itself via the speakers.
func (o *OpenAIRealtime) SendUserAudio(chunk []byte) error {
	if o.speaking.Load() {
		return nil
	}
	return o.send(context.Background(), map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(chunk),
	})
}

func (o *OpenAIRealtime) SendContext(text string) error {
	o.log().Info("send context", "text", text)
	return o.send(context.Background(), map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": "system",
			"content": []map[string]any{
				{"type": "input_text", "text": text},
			},
		},
	})
}

func (o *OpenAIRealtime) TriggerResponse(instructions string) error {
	// Skip if a response is already streaming — OpenAI rejects with
	// conversation_already_has_active_response. The speaking flag is
	// cleared ~speakerDrain after response.done.
	if o.speaking.Load() {
		o.log().Debug("trigger skipped: coach still speaking")
		return nil
	}
	o.log().Info("trigger response", "instructions", instructions)
	payload := map[string]any{"type": "response.create"}
	if instructions != "" {
		payload["response"] = map[string]any{"instructions": instructions}
	}
	return o.send(context.Background(), payload)
}

func (o *OpenAIRealtime) AudioOut() <-chan []byte   { return o.audioOut }
func (o *OpenAIRealtime) Transcript() <-chan string { return o.transcript }

func (o *OpenAIRealtime) Close() error {
	if o.cancel != nil {
		o.cancel()
	}
	if o.conn != nil {
		return o.conn.Close(websocket.StatusNormalClosure, "")
	}
	return nil
}

func (o *OpenAIRealtime) readLoop(ctx context.Context) {
	log := o.log()
	for {
		_, data, err := o.conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Error("realtime read", "err", err)
			}
			return
		}
		// Raw server events go to DEBUG; LOG_LEVEL=debug to see them.
		if log.Enabled(ctx, slog.LevelDebug) {
			log.Debug("recv raw", "bytes", len(data), "payload", string(data))
		}
		var evt struct {
			Type       string `json:"type"`
			Delta      string `json:"delta"`
			Transcript string `json:"transcript"`
			Error      struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "response.created":
			o.speaking.Store(true)
		case "response.done", "response.cancelled":
			time.AfterFunc(speakerDrain, func() { o.speaking.Store(false) })
		case "response.audio.delta", "response.output_audio.delta":
			audio, err := base64.StdEncoding.DecodeString(evt.Delta)
			if err != nil {
				continue
			}
			select {
			case o.audioOut <- audio:
			case <-ctx.Done():
				return
			}
		case "response.audio_transcript.delta", "response.output_audio_transcript.delta":
			select {
			case o.transcript <- evt.Delta:
			case <-ctx.Done():
				return
			}
		case "response.audio_transcript.done", "response.output_audio_transcript.done":
			select {
			case o.transcript <- "\n":
			case <-ctx.Done():
				return
			}
		case "conversation.item.input_audio_transcription.completed":
			log.Info("whisper", "text", evt.Transcript)
		case "conversation.item.input_audio_transcription.failed":
			log.Error("whisper failed", "err", evt.Error.Message)
		case "error":
			log.Error("realtime error", "type", evt.Error.Type, "code", evt.Error.Code, "msg", evt.Error.Message)
		}
	}
}
