// Stadium Sentinel - Self-Healing Infrastructure Agent
// Uses Google ADK with real Gemini LLM for intelligent healing decisions.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"stadium-sentinel/internal/api"
	"stadium-sentinel/internal/monitor"
	"stadium-sentinel/internal/state"
	"stadium-sentinel/internal/tools"
	"stadium-sentinel/internal/whatsapp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	// Google ADK — correct imports per official docs
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

//go:embed dashboard/dist
var staticFiles embed.FS

func initTracer() {
	exporter, _ := stdouttrace.New(stdouttrace.WithPrettyPrint())
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(tp)
}

func main() {
	fmt.Println("=== Stadium Sentinel V2 — Starting ===")
	initTracer()
	tracer := otel.Tracer("stadium-sentinel")
	ctx := context.Background()

	// ── 1. State Registry ─────────────────────────────────────
	registry := state.NewRegistry()

	// ── 2. WhatsApp Client ────────────────────────────────────
	fmt.Println("[Boot] Initializing WhatsApp Bridge...")
	waClient, err := whatsapp.NewClient()
	if err != nil {
		log.Fatalf("[Boot] WhatsApp initialization failed: %v", err)
	}
	defer waClient.Disconnect()
	fmt.Println("[Boot] WhatsApp Bridge ready.")

	// ── 3. Build ADK Tools ────────────────────────────────────
	healerTool, err := functiontool.New(functiontool.Config{
		Name:        "RestartService",
		Description: "Attempts to restart a named stadium service. Call this when a service is detected as DOWN. Returns whether the restart succeeded.",
	}, tools.HealService(registry))
	if err != nil {
		log.Fatalf("[Boot] Failed to create healer tool: %v", err)
	}

	alertTool, err := functiontool.New(functiontool.Config{
		Name:        "EscalateToWhatsApp",
		Description: "Sends a critical escalation alert via WhatsApp. Call this ONLY after RestartService has failed 3 times and the service is still DOWN.",
	}, tools.EscalateToWhatsApp(waClient))
	if err != nil {
		log.Fatalf("[Boot] Failed to create alert tool: %v", err)
	}

	// ── 4. Initialize Gemini Model ────────────────────────────
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("[Boot] GOOGLE_API_KEY environment variable is required")
	}

	model, err := gemini.NewModel(ctx, "gemini-2.0-flash", &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("[Boot] Failed to create Gemini model: %v", err)
	}

	// ── 5. Build ADK LLM Agent (official pattern from adk.dev) ──────
	sentinelAgent, err := llmagent.New(llmagent.Config{
		Name:  "StadiumSentinelAgent",
		Model: model,
		Description: "An intelligent stadium infrastructure guardian that monitors services and self-heals them.",
		Instruction: `You are Stadium Sentinel, an autonomous infrastructure healer.
When you receive an alert that a service is DOWN:
1. IMMEDIATELY call RestartService with the service name.
2. If it succeeds (service is UP), you are done.
3. If it fails, retry up to 2 more times.
4. After 3 failed attempts, call EscalateToWhatsApp with the admin phone number. Always be concise.`,
		Tools: []adktool.Tool{healerTool, alertTool},
	})
	if err != nil {
		log.Fatalf("[Boot] Failed to create ADK agent: %v", err)
	}

	adkConfig := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(sentinelAgent),
	}

	// ── 6. Monitor Engine ─────────────────────────────────────
	fmt.Println("[Boot] Starting Monitoring Engine...")
	monitorEngine := monitor.NewEngine(registry, 100)
	go monitorEngine.Start(ctx)

	// Inject a demo failure 2s after boot
	go func() {
		time.Sleep(2 * time.Second)
		monitorEngine.SimulateFailure("VIP Wi-Fi")
	}()

	// ── 7. HTTP Server (React UI + REST API + SSE) ────────────
	api.StaticFiles = staticFiles
	adminPhone := os.Getenv("ADMIN_PHONE") // Can be empty initially

	apiServer := api.NewServer(registry, monitorEngine, waClient, adminPhone)
	mux := http.NewServeMux()
	apiServer.RegisterRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		fmt.Printf("[Boot] HTTP server on :%s → http://localhost:%s\n", port, port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatalf("[HTTP] Server failed: %v", err)
		}
	}()

	// ── 8. Orchestrator Event Loop (ADK agent makes decisions) ─
	fmt.Printf("[Boot] Agent online. Waiting for setup or failure events...\n")
	fmt.Println("──────────────────────────────────────────────────────────")

	for evt := range monitorEngine.Events() {
		if evt.Type != monitor.EventServiceDown {
			continue
		}

		_, span := tracer.Start(ctx, "AgentHealingDecision")
		logMsg := fmt.Sprintf("SERVICE DOWN: %s at %s", evt.ServiceName, evt.Timestamp.Format("15:04:05"))
		fmt.Printf("\n[ALERT] %s\n", logMsg)
		apiServer.BroadcastEvent(marshalEvent("SERVICE_DOWN", evt.ServiceName, logMsg))

		currentPhone := apiServer.AdminPhone()
		if currentPhone == "" {
			fmt.Println("[Agent] Skipping healing/escalation because no admin phone is configured.")
			span.End()
			continue
		}

		fmt.Printf("[Agent] Gemini is analyzing '%s' and deciding actions...\n", evt.ServiceName)
		
		prompt := fmt.Sprintf(
			"CRITICAL: The '%s' service is DOWN. Admin phone: %s. Please heal it now.",
			evt.ServiceName,
			currentPhone,
		)

		// Create a dynamic CLI slice to execute the specific prompt instantly 
		// via the official ADK launcher pattern.
		args := []string{"--prompt", prompt}
		l := full.NewLauncher()

		fmt.Printf("[Agent] 🤖 Running ADK Launcher with prompt for %s...\n", evt.ServiceName)
		if err = l.Execute(ctx, adkConfig, args); err != nil {
			fmt.Printf("[Agent] Execution blocked/error: %v\n", err)
		} else {
			// Broadcast completion
			// The tools themselves will broadcast intermediate HEALING/ESCALATED logs 
			// if we pass apiServer into the tool, but for now we see terminal output.
			apiServer.BroadcastEvent(marshalEvent("HEALING_COMPLETE", evt.ServiceName, "Check agent logs for results."))
		}

		span.End()
		fmt.Println("──────────────────────────────────────────────────────────")
	}
}

// marshalEvent helper for the API server to format SSE messages
func marshalEvent(eventType, svc, msg string) string {
	b, _ := json.Marshal(map[string]string{
		"type":    eventType,
		"service": svc,
		"message": msg,
		"time":    time.Now().Format(time.RFC3339),
	})
	return string(b)
}
