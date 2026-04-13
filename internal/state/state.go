package state

import "sync"

type ServiceStatus string

const (
	StatusUp   ServiceStatus = "UP"
	StatusDown ServiceStatus = "DOWN"
)

type StadiumService struct {
	Name    string        `json:"name"`
	Status  ServiceStatus `json:"status"`
	Retries int           `json:"retries"`
}

type Registry struct {
	mu       sync.RWMutex
	services map[string]*StadiumService
}

func NewRegistry() *Registry {
	return &Registry{
		services: map[string]*StadiumService{
			"Ticketing API":  {Name: "Ticketing API", Status: StatusUp},
			"VIP Wi-Fi":      {Name: "VIP Wi-Fi", Status: StatusUp},
			"Food Court POS": {Name: "Food Court POS", Status: StatusUp},
		},
	}
}

// SetStatus updates the status of a service. Fixed: was calling Lock() in defer instead of Unlock().
func (r *Registry) SetStatus(name string, status ServiceStatus) {
	r.mu.Lock()
	defer r.mu.Unlock() // BUG FIX: was `defer r.mu.Lock()` — would have caused a deadlock
	if s, ok := r.services[name]; ok {
		s.Status = status
	}
}

func (r *Registry) GetStatus(name string) ServiceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.services[name]; ok {
		return s.Status
	}
	return StatusDown
}

func (r *Registry) GetAll() map[string]*StadiumService {
	r.mu.RLock()
	defer r.mu.RUnlock()

	copyMap := make(map[string]*StadiumService)
	for k, v := range r.services {
		copyMap[k] = &StadiumService{
			Name:    v.Name,
			Status:  v.Status,
			Retries: v.Retries,
		}
	}
	return copyMap
}

func (r *Registry) IncrementRetries(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.services[name]; ok {
		s.Retries++
		return s.Retries
	}
	return 0
}

func (r *Registry) ResetRetries(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.services[name]; ok {
		s.Retries = 0
	}
}
