package auth

import "net/http"

// Operation identifies a protected business capability. Keeping the policy keyed
// by capability makes authorization changes explicit and reviewable instead of
// scattering role lists throughout the HTTP route registration.
type Operation string

const (
	OperationManageTenants   Operation = "manage_tenants"
	OperationWriteLibraries  Operation = "write_libraries"
	OperationWriteQuota      Operation = "write_quota"
	OperationManageUsers     Operation = "manage_users"
	OperationManageTeams     Operation = "manage_teams"
	OperationWriteAssets     Operation = "write_assets"
	OperationReviewAssets    Operation = "review_assets"
	OperationValidateAssets  Operation = "validate_assets"
	OperationWriteChannels   Operation = "write_channels"
	OperationControlChannels Operation = "control_channels"
	OperationIssuePlayback   Operation = "issue_playback"
	OperationWriteSchedule   Operation = "write_schedule"
	OperationReprocessMedia  Operation = "reprocess_media"
)

var operationRoles = map[Operation][]string{
	OperationManageTenants:   {RoleAdmin},
	OperationWriteLibraries:  {RoleAdmin, RoleEditor},
	OperationWriteQuota:      {RoleAdmin},
	OperationManageUsers:     {RoleAdmin},
	OperationManageTeams:     {RoleAdmin},
	OperationWriteAssets:     {RoleAdmin, RoleEditor},
	OperationReviewAssets:    {RoleAdmin, RoleEditor},
	OperationValidateAssets:  {RoleAdmin, RoleEditor},
	OperationWriteChannels:   {RoleAdmin, RoleEditor},
	OperationControlChannels: {RoleAdmin, RoleOperator},
	OperationIssuePlayback:   {RoleAdmin, RoleOperator},
	OperationWriteSchedule:   {RoleAdmin, RoleEditor},
	OperationReprocessMedia:  {RoleAdmin, RoleOperator},
}

// RolesForOperation returns a copy so callers cannot mutate the policy.
func RolesForOperation(operation Operation) []string {
	roles := operationRoles[operation]
	return append([]string(nil), roles...)
}

// RequireOperation applies the centrally-defined role policy to a route.
// Unknown operations are programming errors and fail during route setup rather
// than silently creating an unprotected endpoint.
func RequireOperation(operation Operation) func(http.Handler) http.Handler {
	roles, ok := operationRoles[operation]
	if !ok {
		panic("unknown authorization operation: " + string(operation))
	}
	return RequireRoles(roles...)
}
