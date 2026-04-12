package service

import "context"

// EventSink pushes UI events (e.g. Wails EventsEmit). Optional; nil is safe.
type EventSink interface {
	Emit(ctx context.Context, event string, payload map[string]interface{})
}

type noopEventSink struct{}

func (noopEventSink) Emit(context.Context, string, map[string]interface{}) {}
