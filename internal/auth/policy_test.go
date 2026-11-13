package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizationPolicyMatrix(t *testing.T) {
	tests := []struct {
		operation Operation
		allowed   []string
	}{
		{OperationManageTenants, []string{RoleAdmin}},
		{OperationWriteLibraries, []string{RoleAdmin, RoleEditor}},
		{OperationWriteQuota, []string{RoleAdmin}},
		{OperationManageUsers, []string{RoleAdmin}},
		{OperationManageTeams, []string{RoleAdmin}},
		{OperationWriteAssets, []string{RoleAdmin, RoleEditor}},
		{OperationReviewAssets, []string{RoleAdmin, RoleEditor}},
		{OperationValidateAssets, []string{RoleAdmin, RoleEditor}},
		{OperationWriteChannels, []string{RoleAdmin, RoleEditor}},
		{OperationControlChannels, []string{RoleAdmin, RoleOperator}},
		{OperationIssuePlayback, []string{RoleAdmin, RoleOperator}},
		{OperationWriteSchedule, []string{RoleAdmin, RoleEditor}},
		{OperationReprocessMedia, []string{RoleAdmin, RoleOperator}},
	}

	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			got := RolesForOperation(tt.operation)
			if len(got) != len(tt.allowed) {
				t.Fatalf("roles = %v, want %v", got, tt.allowed)
			}
			for i := range tt.allowed {
				if got[i] != tt.allowed[i] {
					t.Fatalf("roles = %v, want %v", got, tt.allowed)
				}
			}
			got[0] = "mutated"
			if RolesForOperation(tt.operation)[0] == "mutated" {
				t.Fatal("policy roles can be mutated by callers")
			}
		})
	}
}

func TestRequireOperationEnforcesMatrix(t *testing.T) {
	operations := []Operation{
		OperationManageTenants,
		OperationWriteLibraries,
		OperationWriteQuota,
		OperationManageUsers,
		OperationManageTeams,
		OperationWriteAssets,
		OperationReviewAssets,
		OperationValidateAssets,
		OperationWriteChannels,
		OperationControlChannels,
		OperationIssuePlayback,
		OperationWriteSchedule,
		OperationReprocessMedia,
	}
	roles := []string{RoleAdmin, RoleEditor, RoleOperator, RoleViewer}
	for _, operation := range operations {
		allowed := make(map[string]bool)
		for _, role := range RolesForOperation(operation) {
			allowed[role] = true
		}
		for _, role := range roles {
			want := http.StatusForbidden
			if allowed[role] {
				want = http.StatusNoContent
			}
			t.Run(string(operation)+"/"+role, func(t *testing.T) {
				next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				})
				req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
				req = req.WithContext(withPrincipal(req.Context(), Principal{Role: role}))
				res := httptest.NewRecorder()
				RequireOperation(operation)(next).ServeHTTP(res, req)
				if res.Code != want {
					t.Fatalf("status = %d, want %d", res.Code, want)
				}
			})
		}
	}
}

// withPrincipal keeps policy tests focused on authorization without requiring a
// signed token or a database-backed membership lookup.
func withPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}
