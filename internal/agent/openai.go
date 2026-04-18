package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/coder/websocket"
)

const realtimeBaseURL = "wss://api.openai.com/v1/realtime"

type OpenAIRealtime struct {
	model      string
	debug      bool
	conn       *websocket.Conn
	writeMu    sync.Mutex
	audioOut   chan []byte
	transcript chan string
	cancel     context.CancelFunc
}

func NewOpenAIRealtime(model string, debug bool) *OpenAIRealtime {
	return &OpenAIRealtime{
		model:      model,
		debug:      debug,
		audioOut:   make(chan []byte, 64),
		transcript: make(chan string, 64),
	}
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

func (o *OpenAIRealtime) SendUserAudio(chunk []byte) error {
	return o.send(context.Background(), map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(chunk),
	})
}

func (o *OpenAIRealtime) SendContext(text string) error {
	if err := o.send(context.Background(), map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": "system",
			"content": []map[string]any{
				{"type": "input_text", "text": text},
			},
		},
	}); err != nil {
		return err
	}
	return o.send(context.Background(), map[string]any{"type": "response.create"})
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
	for {
		_, data, err := o.conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("realtime read: %v", err)
			}
			return
		}
		if o.debug {
			snippet := string(data)
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			log.Printf("recv: %s", snippet)
		}
		var evt struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Error struct {
				Type    string `json:"type"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &evt); err != nil {
			continue
		}
		switch evt.Type {
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
		case "error":
			log.Printf("realtime error: %s (%s/%s)", evt.Error.Message, evt.Error.Type, evt.Error.Code)
		}
	}
}
