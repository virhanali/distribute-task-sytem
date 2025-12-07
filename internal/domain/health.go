package domain

type HealthStatus struct {
	Database string `json:"database"`
	System   string `json:"system"`
}
