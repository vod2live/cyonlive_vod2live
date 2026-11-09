package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
)

var (
	ErrBootstrapClosed  = errors.New("bootstrap already completed")
	ErrNotFound         = errors.New("not found")
	ErrLastAdmin        = errors.New("organization must retain an administrator")
	ErrAlreadyMember    = errors.New("user is already a member")
	ErrPasswordRequired = errors.New("password required for a new user")
)

type Repository struct{ Pool *pgxpool.Pool }

type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Role  string    `json:"role"`
}

type Organization struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
	Role string    `json:"role"`
}

type Team struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
}

func (r Repository) Bootstrap(ctx context.Context, email, passwordHash, organizationName string) (uuid.UUID, uuid.UUID, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(140014)`); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	var userCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if userCount != 0 {
		return uuid.Nil, uuid.Nil, ErrBootstrapClosed
	}
	var organizationID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug='legacy' FOR UPDATE`).Scan(&organizationID); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("legacy organization missing: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE organizations SET name=$2 WHERE id=$1`, organizationID, organizationName); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	var userID uuid.UUID
	err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES(lower($1),$2) RETURNING id`, email, passwordHash).Scan(&userID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role) VALUES($1,$2,$3)`, organizationID, userID, auth.RoleAdmin); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return userID, organizationID, nil
}

func (r Repository) UserByEmail(ctx context.Context, email string) (uuid.UUID, string, bool, error) {
	var id uuid.UUID
	var hash string
	var active bool
	err := r.Pool.QueryRow(ctx, `SELECT id,password_hash,active FROM users WHERE lower(email)=lower($1)`, email).Scan(&id, &hash, &active)
	return id, hash, active, err
}

func (r Repository) Memberships(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	rows, err := r.Pool.Query(ctx, `SELECT o.id,o.name,o.slug,m.role FROM organization_memberships m JOIN organizations o ON o.id=m.organization_id WHERE m.user_id=$1 ORDER BY o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Organization
	for rows.Next() {
		var item Organization
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Role); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r Repository) RoleForUser(ctx context.Context, userID, organizationID uuid.UUID) (string, error) {
	var role string
	err := r.Pool.QueryRow(ctx, `SELECT m.role FROM organization_memberships m JOIN users u ON u.id=m.user_id WHERE m.user_id=$1 AND m.organization_id=$2 AND u.active=true`, userID, organizationID).Scan(&role)
	return role, err
}

func (r Repository) CreateOrganization(ctx context.Context, userID uuid.UUID, name, slug string) (Organization, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer tx.Rollback(ctx)
	var item Organization
	err = tx.QueryRow(ctx, `INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id,name,slug`, name, slug).Scan(&item.ID, &item.Name, &item.Slug)
	if err != nil {
		return Organization{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role) VALUES($1,$2,$3)`, item.ID, userID, auth.RoleAdmin); err != nil {
		return Organization{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Organization{}, err
	}
	item.Role = auth.RoleAdmin
	return item, nil
}

func (r Repository) ListOrganizations(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	return r.Memberships(ctx, userID)
}

func (r Repository) CreateUser(ctx context.Context, organizationID uuid.UUID, email, passwordHash, role string) (User, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var item User
	err = tx.QueryRow(ctx, `SELECT id,email FROM users WHERE lower(email)=lower($1) FOR UPDATE`, email).Scan(&item.ID, &item.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		if passwordHash == "" {
			return User{}, ErrPasswordRequired
		}
		err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES(lower($1),$2) RETURNING id,email`, email, passwordHash).Scan(&item.ID, &item.Email)
	}
	if err != nil {
		return User{}, err
	}
	var alreadyMember bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2)`, organizationID, item.ID).Scan(&alreadyMember); err != nil {
		return User{}, err
	}
	if alreadyMember {
		return User{}, ErrAlreadyMember
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role) VALUES($1,$2,$3)`, organizationID, item.ID, role); err != nil {
		return User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, err
	}
	item.Role = role
	return item, nil
}

func (r Repository) ListUsers(ctx context.Context, organizationID uuid.UUID) ([]User, error) {
	rows, err := r.Pool.Query(ctx, `SELECT u.id,u.email,m.role FROM organization_memberships m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 ORDER BY u.email`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []User
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Email, &item.Role); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r Repository) UpdateRole(ctx context.Context, organizationID, userID uuid.UUID, role string) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID); err != nil {
		return err
	}
	var current string
	err = tx.QueryRow(ctx, `SELECT role FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 FOR UPDATE`, organizationID, userID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current == auth.RoleAdmin && role != auth.RoleAdmin {
		var administrators int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND role=$2 AND user_id<>$3`, organizationID, auth.RoleAdmin, userID).Scan(&administrators); err != nil {
			return err
		}
		if administrators == 0 {
			return ErrLastAdmin
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE organization_memberships SET role=$3 WHERE organization_id=$1 AND user_id=$2`, organizationID, userID, role); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r Repository) RemoveUser(ctx context.Context, organizationID, userID uuid.UUID) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, organizationID); err != nil {
		return err
	}
	var role string
	err = tx.QueryRow(ctx, `SELECT role FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 FOR UPDATE`, organizationID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == auth.RoleAdmin {
		var administrators int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND role=$2 AND user_id<>$3`, organizationID, auth.RoleAdmin, userID).Scan(&administrators); err != nil {
			return err
		}
		if administrators == 0 {
			return ErrLastAdmin
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM organization_memberships WHERE organization_id=$1 AND user_id=$2`, organizationID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r Repository) CreateTeam(ctx context.Context, organizationID uuid.UUID, name string) (Team, error) {
	var item Team
	err := r.Pool.QueryRow(ctx, `INSERT INTO teams(organization_id,name) VALUES($1,$2) RETURNING id,organization_id,name`, organizationID, name).Scan(&item.ID, &item.OrganizationID, &item.Name)
	return item, err
}

func (r Repository) ListTeams(ctx context.Context, organizationID uuid.UUID) ([]Team, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,organization_id,name FROM teams WHERE organization_id=$1 ORDER BY name`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Team
	for rows.Next() {
		var item Team
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Name); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r Repository) AddTeamMember(ctx context.Context, organizationID, teamID, userID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO team_memberships(organization_id,team_id,user_id) VALUES($1,$2,$3)`, organizationID, teamID, userID)
	return err
}

func (r Repository) RemoveTeamMember(ctx context.Context, organizationID, teamID, userID uuid.UUID) error {
	result, err := r.Pool.Exec(ctx, `DELETE FROM team_memberships WHERE organization_id=$1 AND team_id=$2 AND user_id=$3`, organizationID, teamID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r Repository) Audit(ctx context.Context, organizationID, userID uuid.UUID, action, resource string, status int, requestID string) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO audit_events(organization_id,actor_user_id,action,resource,http_status,request_id) VALUES($1,$2,$3,$4,$5,$6)`, organizationID, userID, action, strings.TrimSpace(resource), status, requestID)
	return err
}

func (r Repository) IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r Repository) IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
