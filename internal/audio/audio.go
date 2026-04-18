package audio

import (
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
)

const (
	SampleRate  = 24000
	Channels    = 1
	BitDepth    = 16
	BytesPerSec = SampleRate * Channels * (BitDepth / 8)
)

// IO owns a capture device (mic) and a playback device (speaker), both 24kHz mono PCM16.
type IO struct {
	ctx      *malgo.AllocatedContext
	capture  *malgo.Device
	playback *malgo.Device

	InCh chan []byte // mic frames → consumer
	muted bool
	muteMu sync.RWMutex

	playBuf   []byte
	playBufMu sync.Mutex
}

func New() (*IO, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(msg string) {
		// silence malgo log spam; uncomment for debugging:
		// fmt.Fprintln(os.Stderr, "malgo:", msg)
	})
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}

	io := &IO{
		ctx:  ctx,
		InCh: make(chan []byte, 64),
	}

	capConf := malgo.DefaultDeviceConfig(malgo.Capture)
	capConf.Capture.Format = malgo.FormatS16
	capConf.Capture.Channels = Channels
	capConf.SampleRate = SampleRate
	capConf.Alsa.NoMMap = 1

	capture, err := malgo.InitDevice(ctx.Context, capConf, malgo.DeviceCallbacks{
		Data: func(_ []byte, inputSamples []byte, _ uint32) {
			io.muteMu.RLock()
			muted := io.muted
			io.muteMu.RUnlock()
			if muted {
				return
			}
			buf := make([]byte, len(inputSamples))
			copy(buf, inputSamples)
			select {
			case io.InCh <- buf:
			default:
				// drop if consumer is behind
			}
		},
	})
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("capture init: %w", err)
	}
	io.capture = capture

	plyConf := malgo.DefaultDeviceConfig(malgo.Playback)
	plyConf.Playback.Format = malgo.FormatS16
	plyConf.Playback.Channels = Channels
	plyConf.SampleRate = SampleRate

	playback, err := malgo.InitDevice(ctx.Context, plyConf, malgo.DeviceCallbacks{
		Data: func(outputSamples []byte, _ []byte, _ uint32) {
			io.fillPlayback(outputSamples)
		},
	})
	if err != nil {
		capture.Uninit()
		ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("playback init: %w", err)
	}
	io.playback = playback

	return io, nil
}

func (io *IO) Start() error {
	if err := io.capture.Start(); err != nil {
		return fmt.Errorf("capture start: %w", err)
	}
	if err := io.playback.Start(); err != nil {
		return fmt.Errorf("playback start: %w", err)
	}
	return nil
}

// Play enqueues a PCM16 chunk for playback. Returns immediately.
func (io *IO) Play(chunk []byte) {
	io.playBufMu.Lock()
	io.playBuf = append(io.playBuf, chunk...)
	io.playBufMu.Unlock()
}

func (io *IO) fillPlayback(out []byte) {
	io.playBufMu.Lock()
	defer io.playBufMu.Unlock()
	n := copy(out, io.playBuf)
	io.playBuf = io.playBuf[n:]
	for i := n; i < len(out); i++ {
		out[i] = 0
	}
}

// SetMuted controls whether mic frames are sent to InCh.
func (io *IO) SetMuted(m bool) {
	io.muteMu.Lock()
	io.muted = m
	io.muteMu.Unlock()
}

// FlushPlayback drops any buffered speaker audio (e.g., when the user barges in).
func (io *IO) FlushPlayback() {
	io.playBufMu.Lock()
	io.playBuf = io.playBuf[:0]
	io.playBufMu.Unlock()
}

func (io *IO) Close() {
	if io.capture != nil {
		io.capture.Uninit()
	}
	if io.playback != nil {
		io.playback.Uninit()
	}
	if io.ctx != nil {
		io.ctx.Uninit()
		io.ctx.Free()
	}
}
