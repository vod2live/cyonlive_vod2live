package asset

import "github.com/google/uuid"

type Status string

type ApprovalStatus string

const (
	StatusPending Status = "pending"
	StatusValid   Status = "valid"
	StatusInvalid Status = "invalid"

	ApprovalNotRequired ApprovalStatus = "not_required"
	ApprovalPending     ApprovalStatus = "pending"
	ApprovalApproved    ApprovalStatus = "approved"
	ApprovalRejected    ApprovalStatus = "rejected"
)

type Asset struct {
	ID             uuid.UUID      `json:"id"`
	OrganizationID uuid.UUID      `json:"organization_id"`
	LibraryID      *uuid.UUID     `json:"library_id,omitempty"`
	TenantID       *uuid.UUID     `json:"tenant_id,omitempty"`
	Title          string         `json:"title"`
	HLSURL         string         `json:"hls_url"`
	Status         Status         `json:"status"`
	ApprovalStatus ApprovalStatus `json:"approval_status"`
	DurationMS     int64          `json:"duration_ms,omitempty"`
	Metadata       any            `json:"metadata,omitempty"`
}

// EligibleForChannel enforces the application-level rule; persistence also
// applies the same condition when creating or enabling schedule entries.
func EligibleForChannel(item Asset, channelTenantID *uuid.UUID) bool {
	if item.Status != StatusValid {
		return false
	}
	if item.LibraryID != nil && item.ApprovalStatus != ApprovalApproved {
		return false
	}
	return item.TenantID == nil || channelTenantID != nil && *item.TenantID == *channelTenantID
}

type Repository interface {
	Create(asset Asset) error
	FindByID(id uuid.UUID) (Asset, error)
	UpdateStatus(id uuid.UUID, status Status, durationMS int64, metadata any) error
}
