package daemon

import (
	"sync"

	"agentscale/pkg/runtime"
	"agentscale/pkg/scaling"
)

// channelStreamWriter adapts runtime.StreamWriter to channel-based streaming.
// It bridges the gap between daemon's ExecuteStreaming (which expects runtime.StreamWriter)
// and the worker pool's channel-based communication (chan *scaling.StreamEvent).
type channelStreamWriter struct {
	ch     chan *scaling.StreamEvent
	mu     sync.Mutex
	closed bool
}

// newChannelStreamWriter creates a new channel-based stream writer.
func newChannelStreamWriter(ch chan *scaling.StreamEvent) *channelStreamWriter {
	return &channelStreamWriter{
		ch: ch,
	}
}

// WriteEvent implements runtime.StreamWriter by converting and sending to channel.
func (w *channelStreamWriter) WriteEvent(event *runtime.StreamEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.ch == nil {
		return nil // Silent drop if channel closed or nil
	}

	// Convert runtime.StreamEvent → scaling.StreamEvent
	scalingEvent := &scaling.StreamEvent{
		Type:      event.Type,
		Timestamp: event.Timestamp,
		Data:      event.Data,
	}

	// Non-blocking send (drops if handler disconnected/slow)
	select {
	case w.ch <- scalingEvent:
		return nil
	default:
		// Channel full or receiver slow - drop event
		// This is acceptable for SSE (client can't keep up)
		return nil
	}
}

// Close implements runtime.StreamWriter.
func (w *channelStreamWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}
