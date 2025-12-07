package health

import (
	"net/http"
)

type HealthHandlerInterface interface {
	Check(w http.ResponseWriter, r *http.Request)
}
