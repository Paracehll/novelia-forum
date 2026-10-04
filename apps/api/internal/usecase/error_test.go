package usecase

import (
	"errors"
	"fmt"
	"testing"
)

func TestAppErrorConstructors(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(string, string) *AppError
		kind ErrorKind
	}{
		{"invalid", Invalid, KindInvalid},
		{"not found", NotFound, KindNotFound},
		{"conflict", Conflict, KindConflict},
		{"forbidden", Forbidden, KindForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.make("test.code", "测试消息")
			if err.Kind != tc.kind || err.Code != "test.code" || err.Message != "测试消息" || err.Cause != nil {
				t.Fatalf("unexpected application error: %+v", err)
			}
		})
	}
}
func TestAppError(t *testing.T) {
	cause := errors.New("database unavailable")
	err := &AppError{
		Kind:    KindConflict,
		Code:    "comment.conflict",
		Message: "评论数据冲突",
		Cause:   cause,
	}
	if err.Error() != err.Message {
		t.Fatalf("Error() = %q, want %q", err.Error(), err.Message)
	}
	if !errors.Is(err, cause) {
		t.Fatal("AppError did not expose its diagnostic cause")
	}
	var appErr *AppError
	wrapped := fmt.Errorf("operation failed: %w", err)
	if !errors.As(wrapped, &appErr) {
		t.Fatal("wrapped error did not expose AppError")
	}
	if appErr.Kind != KindConflict || appErr.Code != "comment.conflict" {
		t.Fatalf("unexpected application error: %+v", appErr)
	}
}
