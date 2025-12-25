package orgs

import (
	"time"

	"github.com/google/uuid"
)

// OrgRole represents a user's role within an organization
type OrgRole string

const (
	RoleOwner  OrgRole = "OWNER"
	RoleAdmin  OrgRole = "ADMIN"
	RoleMember OrgRole = "MEMBER"
	RoleViewer OrgRole = "VIEWER"
)

// CanMutate returns true if the role has permission to modify organization resources
func (r OrgRole) CanMutate() bool {
	return r == RoleOwner || r == RoleAdmin
}

// Org represents an organization in the system
type Org struct {
	ID              uuid.UUID `db:"id"`
	Name            string    `db:"name"`
	Slug            string    `db:"slug"`
	CreatedByUserID uuid.UUID `db:"created_by_user_id"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// Membership represents a user's membership in an organization
type Membership struct {
	OrgID     uuid.UUID `db:"org_id"`
	UserID    uuid.UUID `db:"user_id"`
	Role      OrgRole   `db:"role"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// OrgWithRole combines org information with the user's role
type OrgWithRole struct {
	Org
	Role OrgRole `db:"role"`
}

// MemberInfo represents a member of an organization with their details
type MemberInfo struct {
	UserID    uuid.UUID `db:"user_id" json:"user_id"`
	Email     string    `db:"email" json:"email"`
	Role      OrgRole   `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
