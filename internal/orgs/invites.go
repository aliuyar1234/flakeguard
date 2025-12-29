package orgs

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidOrgRole      = errors.New("invalid organization role")
	ErrCannotInviteOwner   = errors.New("cannot invite owner role")
	ErrInviteNotFound      = errors.New("invite not found")
	ErrInviteExpired       = errors.New("invite expired")
	ErrInviteNotActive     = errors.New("invite not active")
	ErrInviteEmailMismatch = errors.New("invite email does not match user")
)

type Invite struct {
	ID        uuid.UUID `db:"id"`
	OrgID     uuid.UUID `db:"org_id"`
	Email     string    `db:"email"`
	Role      OrgRole   `db:"role"`
	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

type InviteListItem struct {
	ID             uuid.UUID `db:"id" json:"id"`
	Email          string    `db:"email" json:"email"`
	Role           OrgRole   `db:"role" json:"role"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	ExpiresAt      time.Time `db:"expires_at" json:"expires_at"`
	CreatedByEmail string    `db:"created_by_email" json:"created_by_email"`
}
