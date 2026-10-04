package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/lib/pq"
)

const (
	StatusPublished int16 = 0
	StatusHidden    int16 = 1
	StatusDeleted   int16 = 2
)

var (
	ErrNotFound            = errors.New("record not found")
	ErrConflict            = errors.New("record conflict")
	ErrCommentRootNotFound = errors.New("comment root not found")
	ErrInvalidCommentRoot  = errors.New("invalid comment root")
	ErrInvalidCategory     = errors.New("invalid category")
	ErrInvalidTag          = errors.New("invalid or inactive tag")
	ErrCommentsLocked      = errors.New("comments are locked")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, qrm.ErrNoRows)
}

func IsUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.Is(err, ErrConflict) || errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// storageError translates known storage outcomes without leaking driver errors.
// Unknown failures retain their cause for logging at the transport boundary.
func storageError(err error, operation string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, qrm.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		err = ErrNotFound
	case IsUniqueViolation(err):
		err = ErrConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}
