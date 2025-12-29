package orgs

import "errors"

var (
	ErrMemberNotFound        = errors.New("member not found")
	ErrCannotDemoteLastOwner = errors.New("cannot demote last owner")
	ErrCannotRemoveLastOwner = errors.New("cannot remove last owner")
)
