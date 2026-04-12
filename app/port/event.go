package port

// EventBus notifies the UI (or other observers) of domain events.
type EventBus interface {
	Emit(event string, payload map[string]interface{})
}
