package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/lib/pq"
)

func TestStorageErrorTranslation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  error
	}{
		{"jet missing", qrm.ErrNoRows, ErrNotFound},
		{"sql missing", sql.ErrNoRows, ErrNotFound},
		{"unique conflict", &pq.Error{Code: "23505", Detail: "private details"}, ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := storageError(fmt.Errorf("query: %w", tc.cause), "comment.Create")
			if !errors.Is(err, tc.want) || errors.Is(err, tc.cause) {
				t.Fatalf("expected stable storage error without driver cause, got %v", err)
			}
			var driverErr *pq.Error
			if errors.As(err, &driverErr) || strings.Contains(err.Error(), "private details") {
				t.Fatalf("driver details escaped repository: %v", err)
			}
		})
	}
	if storageError(nil, "query") != nil {
		t.Fatal("nil error became a failure")
	}
}

func TestStorageErrorPreservesUnknownCause(t *testing.T) {
	cause := &pq.Error{Code: "08006", Message: "connection failure"}
	err := storageError(cause, "comment.Create")
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "comment.Create") {
		t.Fatalf("lost operation or unknown cause: %v", err)
	}
}
