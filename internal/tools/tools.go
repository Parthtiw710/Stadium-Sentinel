package tools

import (
	"fmt"
	"stadium-sentinel/internal/state"
	"stadium-sentinel/internal/whatsapp"

	"google.golang.org/adk/tool"
)

// -- Healer Tool --

type RestartArgs struct {
	ServiceName string `json:"service_name"`
}
type RestartResults struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

func HealService(registry *state.Registry) func(tool.Context, RestartArgs) (RestartResults, error) {
	return func(ctx tool.Context, args RestartArgs) (RestartResults, error) {
		registry.SetStatus(args.ServiceName, state.StatusUp)
		retries := registry.IncrementRetries(args.ServiceName)
		
		return RestartResults{
			Message: fmt.Sprintf("Service %s restarted successfully. Total retries: %d", args.ServiceName, retries),
			Success: true,
		}, nil
	}
}

// -- WhatsApp Trigger Tool --

type AlertArgs struct {
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
}
type AlertResults struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

func EscalateToWhatsApp(client *whatsapp.Client) func(tool.Context, AlertArgs) (AlertResults, error) {
	return func(ctx tool.Context, args AlertArgs) (AlertResults, error) {
		err := client.SendAlert(args.PhoneNumber, args.Message)
		if err != nil {
			return AlertResults{
				Message: fmt.Sprintf("Failed to send alert: %v", err),
				Success: false,
			}, err
		}
		
		return AlertResults{
			Message: fmt.Sprintf("Emergency alert dispatched to %s", args.PhoneNumber),
			Success: true,
		}, nil
	}
}
