package library

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Tenant struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
}

func (r Repository) CreateTenant(ctx context.Context, org uuid.UUID, name, slug string) (Tenant, error) {
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(strings.ToLower(slug))
	if name == "" || slug == "" {
		return Tenant{}, fmt.Errorf("name and slug are required")
	}
	var t Tenant
	err := r.Pool.QueryRow(ctx, `INSERT INTO tenants (organization_id,name,slug) VALUES ($1,$2,$3) RETURNING id,organization_id,name,slug`, org, name, slug).Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Slug)
	return t, err
}

func (r Repository) AddTenantMember(ctx context.Context, org, tenant, user uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO tenant_memberships(tenant_id,organization_id,user_id,role)
		VALUES($1,$2,$3,'administrador') ON CONFLICT (tenant_id,user_id) DO NOTHING`, tenant, org, user)
	return err
}

func (r Repository) ListTenants(ctx context.Context, org uuid.UUID) ([]Tenant, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,organization_id,name,slug FROM tenants WHERE organization_id=$1 ORDER BY name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Slug); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
