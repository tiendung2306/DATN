package wailsui

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"app/service"
)

// EventSink implements service.EventSink using Wails runtime events.
type EventSink struct{}

func NewEventSink() service.EventSink {
	return EventSink{}
}

func (EventSink) Emit(ctx context.Context, event string, payload map[string]interface{}) {
	wailsRuntime.EventsEmit(ctx, event, payload)
}
