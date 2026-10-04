package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"auth/internal/httpx"
	"auth/internal/usecase"
)

func TestCommentErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		err     error
		status  int
		message string
	}{
		{usecase.Invalid("test.invalid", "参数无效"), http.StatusBadRequest, "参数无效"},
		{usecase.Forbidden("test.forbidden", "无权操作"), http.StatusForbidden, "无权操作"},
		{usecase.NotFound("test.not_found", "资源不存在"), http.StatusNotFound, "资源不存在"},
		{usecase.Conflict("test.conflict", "资源冲突"), http.StatusConflict, "资源冲突"},
		{fmt.Errorf("comment.check_subject: %w", errors.New("upstream unavailable")), http.StatusInternalServerError, "服务器内部错误"},
		{fmt.Errorf("comment.list_roots: %w", errors.New("database unavailable")), http.StatusInternalServerError, "服务器内部错误"},
		{errors.New("评论不存在"), http.StatusInternalServerError, "服务器内部错误"},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			cause := errors.New("private database or upstream details")
			err := fmt.Errorf("operation failed: %w: %w", tc.err, cause)
			if !errors.Is(transportError(err), cause) || !errors.Is(transportError(err), tc.err) {
				t.Fatal("transport mapping lost original cause or sentinel")
			}
			response := httptest.NewRecorder()
			response.Header().Set("Content-Length", "999")
			httpx.EH(func(http.ResponseWriter, *http.Request) error { return transportError(err) })(
				response, httptest.NewRequest(http.MethodGet, "/", nil),
			)
			if response.Code != tc.status || response.Body.String() != tc.message {
				t.Fatalf("got (%d, %q), want (%d, %q)", response.Code, response.Body.String(), tc.status, tc.message)
			}
			if response.Header().Get("Content-Length") != "" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("unexpected error headers: %v", response.Header())
			}
		})
	}
}

func TestUnknownAppErrorKindFallsThroughToInternalError(t *testing.T) {
	appErr := &usecase.AppError{Kind: "future", Code: "future.failure", Message: "不应暴露"}
	if transportError(appErr) != appErr {
		t.Fatal("unknown application error kind should remain unhandled")
	}
	response := httptest.NewRecorder()
	httpx.EH(func(http.ResponseWriter, *http.Request) error { return transportError(appErr) })(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusInternalServerError || response.Body.String() != "服务器内部错误" {
		t.Fatalf("got (%d, %q)", response.Code, response.Body.String())
	}
}
