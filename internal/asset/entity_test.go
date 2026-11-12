package asset

import (
	"testing"

	"github.com/google/uuid"
)

func TestEligibleForChannel(t *testing.T) {
	tenantA, tenantB := uuid.New(), uuid.New()
	libraryID := uuid.New()
	tests := []struct {
		name    string
		item    Asset
		channel *uuid.UUID
		want    bool
	}{
		{
			name:    "private tenant asset allowed in same tenant channel",
			item:    Asset{Status: StatusValid, LibraryID: &libraryID, TenantID: &tenantA, ApprovalStatus: ApprovalApproved},
			channel: &tenantA, want: true,
		},
		{
			name:    "private tenant asset denied in another tenant channel",
			item:    Asset{Status: StatusValid, LibraryID: &libraryID, TenantID: &tenantA, ApprovalStatus: ApprovalApproved},
			channel: &tenantB, want: false,
		},
		{
			name: "private tenant asset denied in organization-wide channel",
			item: Asset{Status: StatusValid, LibraryID: &libraryID, TenantID: &tenantA, ApprovalStatus: ApprovalApproved},
			want: false,
		},
		{
			name:    "organization library asset usable by any tenant after approval",
			item:    Asset{Status: StatusValid, LibraryID: &libraryID, ApprovalStatus: ApprovalApproved},
			channel: &tenantB, want: true,
		},
		{
			name: "organization library asset usable in organization-wide channel",
			item: Asset{Status: StatusValid, LibraryID: &libraryID, ApprovalStatus: ApprovalApproved},
			want: true,
		},
		{
			name:    "pending review asset denied",
			item:    Asset{Status: StatusValid, LibraryID: &libraryID, ApprovalStatus: ApprovalPending},
			channel: &tenantA, want: false,
		},
		{
			name:    "rejected asset denied",
			item:    Asset{Status: StatusValid, LibraryID: &libraryID, TenantID: &tenantA, ApprovalStatus: ApprovalRejected},
			channel: &tenantA, want: false,
		},
		{
			name:    "unprocessed asset denied",
			item:    Asset{Status: StatusPending, LibraryID: &libraryID, TenantID: &tenantA, ApprovalStatus: ApprovalApproved},
			channel: &tenantA, want: false,
		},
		{
			name:    "legacy non-library asset remains eligible when valid",
			item:    Asset{Status: StatusValid, ApprovalStatus: ApprovalNotRequired},
			channel: &tenantB, want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EligibleForChannel(tt.item, tt.channel); got != tt.want {
				t.Fatalf("EligibleForChannel() = %v, want %v", got, tt.want)
			}
		})
	}
}
