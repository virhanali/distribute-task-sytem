package health

import (
	"encoding/json"
	"net/http"

	"mirage-boilerplate/internal/service"
)

type HealthHandler struct {
	svc service.HealthService
}

func NewHealthHandler(svc service.HealthService) *HealthHandler {
	return &HealthHandler{
		svc: svc,
	}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	status := h.svc.CheckHealth(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"checks": status,
	})
}
