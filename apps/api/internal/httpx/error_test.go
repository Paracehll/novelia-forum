package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRespondErrorContextErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		ctx    context.Context
		status int
		body   string
	}{
		{"wrapped cancellation", fmt.Errorf("query: %w", context.Canceled), context.Background(), 499, ""},
		{
			"wrapped deadline",
			InternalError(fmt.Errorf("query: %w", context.DeadlineExceeded), "查询失败"),
			context.Background(),
			http.StatusGatewayTimeout,
			"请求超时",
		},
		{"canceled request with driver error", errors.New("driver cancellation"), canceledContext(), 499, ""},
		{
			"expired request with driver error",
			errors.New("driver cancellation"),
			expiredContext(),
			http.StatusGatewayTimeout,
			"请求超时",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			response.Header().Set("Content-Length", "100")
			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tc.ctx)
			EH(func(http.ResponseWriter, *http.Request) error { return tc.err })(response, request)
			if response.Code != tc.status || response.Body.String() != tc.body {
				t.Fatalf("got (%d, %q), want (%d, %q)", response.Code, response.Body.String(), tc.status, tc.body)
			}
			if response.Header().Get("Content-Length") != "" {
				t.Fatal("stale Content-Length retained")
			}
		})
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Time{})
	cancel()
	return ctx
}

func TestRespondErrorHidesUnhandledError(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	respondError(response, request, errors.New("database connection contains secret details"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("RespondError returned status %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if response.Body.String() != "服务器内部错误" {
		t.Fatalf("RespondError returned body %q", response.Body.String())
	}
}

func TestInternalErrorPreservesPublicMessageAndCause(t *testing.T) {
	cause := errors.New("database unavailable")
	err := InternalError(cause, "查询失败")
	if !errors.Is(err, cause) {
		t.Fatal("InternalError did not preserve its cause")
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	respondError(response, request, err)
	if response.Code != http.StatusInternalServerError || response.Body.String() != "查询失败" {
		t.Fatalf("RespondError returned (%d, %q)", response.Code, response.Body.String())
	}
}
