package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"stadium-sentinel/internal/monitor"
	"stadium-sentinel/internal/state"
	"stadium-sentinel/internal/whatsapp"
	"strings"
	"sync"
)

// StaticFiles is populated by the main package via go:embed.
var StaticFiles embed.FS

type sseClient struct {
	id     string
	events chan string
}

type Server struct {
	registry   *state.Registry
	engine     *monitor.Engine
	waClient   *whatsapp.Client
	adminPhone string
	sseClients map[string]*sseClient
	sseMu      sync.RWMutex
}

func NewServer(registry *state.Registry, engine *monitor.Engine, waClient *whatsapp.Client, adminPhone string) *Server {
	return &Server{
		registry:   registry,
		engine:     engine,
		waClient:   waClient,
		adminPhone: adminPhone,
		sseClients: make(map[string]*sseClient),
	}
}

func (s *Server) BroadcastEvent(msg string) {
	s.sseMu.RLock()
	defer s.sseMu.RUnlock()
	for _, client := range s.sseClients {
		select {
		case client.events <- msg:
		default:
		}
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("GET /api/services", s.cors(s.handleGetServices))
	mux.HandleFunc("POST /api/services/{name}/fail", s.cors(s.handleInjectFailure))
	mux.HandleFunc("POST /api/services/{name}/restore", s.cors(s.handleRestoreService))
	mux.HandleFunc("POST /api/setup", s.cors(s.handleSetup))
	mux.HandleFunc("GET /api/events", s.cors(s.handleSSE))
	mux.HandleFunc("GET /api/health", s.cors(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"admin_phone": s.adminPhone,
			"wa_logged_in": s.waClient.IsLoggedIn(),
		})
	}))

	// Serve React SPA from embedded static files
	if StaticFiles != (embed.FS{}) {
		sub, err := fs.Sub(StaticFiles, "dashboard/dist")
		if err == nil {
			fileServer := http.FileServer(http.FS(sub))
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// SPA fallback: serve index.html for non-asset routes
				cleanPath := strings.TrimPrefix(r.URL.Path, "/")
				if cleanPath == "" {
					cleanPath = "."
				}
				
				_, err := sub.Open(cleanPath)
				if err != nil {
					// Serve index.html for client-side routing
					r.URL.Path = "/"
				}
				fileServer.ServeHTTP(w, r)
			})
		}
	}
}

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func (s *Server) SetAdminPhone(phone string) {
	s.adminPhone = phone
}

func (s *Server) AdminPhone() string {
	return s.adminPhone
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.SetAdminPhone(body.Phone)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Setup triggered"})

	// Run WhatsApp login flow in background so it doesn't block the request
	go func() {
		if !s.waClient.IsLoggedIn() {
			err := s.waClient.StartLoginFlow(context.Background(), func(qrCode string) {
				// Broadcast the QR code to the React UI
				s.BroadcastEvent(marshalEvent("WA_QR", "System", qrCode))
			})
			if err != nil {
				s.BroadcastEvent(marshalEvent("WA_ERROR", "System", err.Error()))
				return
			}
		} else {
			// Already logged in, just connect
			s.waClient.Connect()
		}
		// Signal success
		s.BroadcastEvent(marshalEvent("WA_READY", "System", "WhatsApp Authenticated"))
	}()
}

// marshalEvent helper for the API server to format SSE messages
func marshalEvent(eventType, svc, msg string) string {
	b, _ := json.Marshal(map[string]string{
		"type":    eventType,
		"service": svc,
		"message": msg,
	})
	return string(b)
}

func (s *Server) handleGetServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	services := s.registry.GetAll()
	list := make([]*state.StadiumService, 0, len(services))
	for _, svc := range services {
		list = append(list, svc)
	}
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleInjectFailure(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.engine.SimulateFailure(name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("Failure injected for %s", name)})
}

func (s *Server) handleRestoreService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.engine.RestoreService(name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("Service %s restored", name)})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	clientID := fmt.Sprintf("client-%s", r.RemoteAddr)
	client := &sseClient{id: clientID, events: make(chan string, 50)}

	s.sseMu.Lock()
	s.sseClients[clientID] = client
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, clientID)
		s.sseMu.Unlock()
	}()

	for {
		select {
		case msg := <-client.events:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
