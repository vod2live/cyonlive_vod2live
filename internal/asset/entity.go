package asset

import "github.com/google/uuid"

type Status string

const (
	StatusPending Status = "pending"
	StatusValid   Status = "valid"
	StatusInvalid Status = "invalid"
)

type Asset struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Title          string    `json:"title"`
	HLSURL         string    `json:"hls_url"`
	Status         Status    `json:"status"`
	DurationMS     int64     `json:"duration_ms,omitempty"`
	Metadata       any       `json:"metadata,omitempty"`
}

type Repository interface {
	Create(asset Asset) error
	FindByID(id uuid.UUID) (Asset, error)
	UpdateStatus(id uuid.UUID, status Status, durationMS int64, metadata any) error
}
