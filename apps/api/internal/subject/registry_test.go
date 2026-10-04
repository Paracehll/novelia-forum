package subject

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"forum/internal/domain"
)

func registryFor(t *testing.T, base string) *Registry {
	t.Helper()
	registry, err := NewRegistry(novelPlugin(&http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}, base))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestNovelCheckStatus(t *testing.T) {
	for _, status := range []int{204, 400, 404, 200, 401, 403, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()
			got, err := registryFor(t, server.URL).Check(context.Background(), "novel", "web-syosetu-n1234")
			var wantErr error
			if status != http.StatusNoContent && status != http.StatusNotFound {
				wantErr = ErrCheckFailed
			}
			if got != (status == http.StatusNoContent) || err != wantErr {
				t.Fatalf("Check=(%t, %v) for HTTP %d, want error %v", got, err, status, wantErr)
			}
		})
	}
}

func TestNovelRoutesAndEscapesKey(t *testing.T) {
	for _, tc := range []struct{ key, path string }{
		{"web-syosetu-n1234-part-2", "/novel/syosetu/n1234-part-2/exist"},
		{"wenku-507f1f77bcf86cd799439011", "/wenku/507f1f77bcf86cd799439011/exist"},
		{"web-provider-a/b?c#d%", "/novel/provider/a%2Fb%3Fc%23d%25/exist"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.URL.EscapedPath() != tc.path {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			if exists, err := registryFor(t, server.URL).Check(context.Background(), "novel", tc.key); !exists || err != nil {
				t.Fatalf("subject rejected: exists=%t err=%v", exists, err)
			}
		})
	}
}

func TestNovelValidation(t *testing.T) {
	registry := registryFor(t, "http://invalid.invalid")
	for _, key := range []string{"", "unknown-id", "web", "web--id", "web-provider-", "wenku-"} {
		if registry.Valid("novel", key) {
			t.Errorf("key=%q accepted", key)
		}
	}
	if !registry.Valid("novel", "web-syosetu-n1234") {
		t.Fatal("valid novel key rejected")
	}
}

func TestNovelCheckDoesNotFollowRedirect(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()
	if exists, err := registryFor(t, server.URL).Check(context.Background(), "novel", "web-syosetu-n1234"); exists || err != ErrCheckFailed {
		t.Fatalf("redirect result: exists=%t err=%v", exists, err)
	}
	if calls != 1 {
		t.Fatalf("followed redirect: %d requests", calls)
	}
}

func TestNovelCheckTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()
	registry, err := NewRegistry(novelPlugin(&http.Client{Timeout: time.Millisecond}, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := registry.Check(context.Background(), "novel", "web-syosetu-n1234"); exists || err != ErrCheckFailed {
		t.Fatalf("timeout result: exists=%t err=%v", exists, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if exists, err := registry.Check(ctx, "novel", "web-syosetu-n1234"); exists || err != ErrCheckFailed {
		t.Fatalf("cancellation result: exists=%t err=%v", exists, err)
	}
}

func TestNovelCheckFailures(t *testing.T) {
	for _, tc := range []struct{ name, base, key string }{
		{"invalid key", "http://invalid.invalid", "invalid"},
		{"invalid URL", "://invalid", "wenku-book"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exists, err := registryFor(t, tc.base).Check(context.Background(), "novel", tc.key)
			if exists || err != ErrCheckFailed {
				t.Fatalf("Check=(%t, %v), want (false, ErrCheckFailed)", exists, err)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	if exists, err := registryFor(t, server.URL).Check(context.Background(), "novel", "wenku-book"); exists || err != ErrCheckFailed {
		t.Fatalf("network failure result: exists=%t err=%v", exists, err)
	}
}

func TestRegistry(t *testing.T) {
	registry, err := NewRegistry(Novel())
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := registry.Type("novel"); !ok || id != domain.CommentSubjectNovel {
		t.Fatalf("id=%d ok=%v", id, ok)
	}
	if _, ok := registry.Type("unknown"); ok || registry.Valid("unknown", "key") {
		t.Fatal("unknown kind accepted")
	}
	if exists, err := registry.Check(context.Background(), "unknown", "key"); exists || err != ErrCheckFailed {
		t.Fatalf("unknown kind result: exists=%t err=%v", exists, err)
	}
	if _, err := NewRegistry(Novel(), Novel()); err == nil {
		t.Fatal("duplicate kind accepted")
	}
	invalid := Novel()
	invalid.SubjectType = domain.CommentSubjectPost
	if _, err := NewRegistry(invalid); err == nil {
		t.Fatal("post subject type accepted as an external plugin")
	}
}
