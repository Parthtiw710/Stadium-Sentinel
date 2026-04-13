package monitor

import (
	"context"
	"fmt"
	"stadium-sentinel/internal/state"
	"time"
)

type EventType string

const (
	EventServiceDown EventType = "SERVICE_DOWN"
	EventServiceUp   EventType = "SERVICE_UP"
)

type AlertEvent struct {
	ServiceName string
	Type        EventType
	Timestamp   time.Time
}

type Engine struct {
	registry *state.Registry
	events   chan AlertEvent
}

func NewEngine(registry *state.Registry, bufferSize int) *Engine {
	return &Engine{
		registry: registry,
		events:   make(chan AlertEvent, bufferSize),
	}
}

func (e *Engine) Events() <-chan AlertEvent {
	return e.events
}

func (e *Engine) Start(ctx context.Context) {
	services := e.registry.GetAll()
	for name := range services {
		go e.pollService(ctx, name)
	}
}

func (e *Engine) pollService(ctx context.Context, name string) {
	// Use two intervals: fast poll (3s) when UP, slow cooldown (10s) when DOWN
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	inCooldown := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if inCooldown {
				// After cooldown, reset and resume normal polling
				inCooldown = false
				ticker.Reset(3 * time.Second)
				continue
			}

			status := e.registry.GetStatus(name)
			if status == state.StatusDown {
				// Non-blocking send to event channel
				select {
				case e.events <- AlertEvent{
					ServiceName: name,
					Type:        EventServiceDown,
					Timestamp:   time.Now(),
				}:
				default:
					// Channel full, skip this event to avoid blocking the poller goroutine
				}
				// Start cooldown to avoid spamming
				inCooldown = true
				ticker.Reset(10 * time.Second)
			}
		}
	}
}

// SimulateFailure manually injects a failure — can be called safely from HTTP handler.
func (e *Engine) SimulateFailure(name string) {
	fmt.Printf("[Monitor] INJECTING FAILURE: %s\n", name)
	e.registry.SetStatus(name, state.StatusDown)
}

// Restore manually brings a service back online.
func (e *Engine) RestoreService(name string) {
	fmt.Printf("[Monitor] RESTORING SERVICE: %s\n", name)
	e.registry.SetStatus(name, state.StatusUp)
	e.registry.ResetRetries(name)
}
