package handler

import (
	"context"
	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"testing"
)

type blacklistTestRepo struct {
	repository.BlacklistRepository
	owner  int64
	target int64
	items  []domain.BlacklistEntry
}

func (r *blacklistTestRepo) LockUser(_ context.Context, id int64) error { r.owner = id; return nil }
func (r *blacklistTestRepo) List(_ context.Context, id int64) ([]domain.BlacklistEntry, error) {
	r.owner = id
	return r.items, nil
}
func (r *blacklistTestRepo) Add(_ context.Context, entry domain.BlacklistEntry) error {
	r.owner = entry.UserID
	r.target = entry.BlockedUserID
	return nil
}
func (r *blacklistTestRepo) Remove(_ context.Context, owner, target int64) error {
	r.owner = owner
	r.target = target
	return nil
}

func TestBlacklistRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		auth         bool
		status       int
	}{
		{"GET", "/me/blacklist", false, 401},
		{"PUT", "/me/blacklist/42", false, 401},
		{"DELETE", "/me/blacklist/42", false, 401},
		{"GET", "/me/blacklist", true, 200},
		{"PUT", "/me/blacklist/42", true, 204},
		{"DELETE", "/me/blacklist/42", true, 204},
		{"PUT", "/me/blacklist/0", true, 400},
		{"PUT", "/me/blacklist/1", true, 400},
		{"PUT", "/me/blacklist/nope", true, 400},
	} {
		t.Run(tc.method+tc.path+http.StatusText(tc.status), func(t *testing.T) {
			repo := &blacklistTestRepo{}
			router := chi.NewRouter()
			router.Route("/me", NewMeHandler(nil, usecase.NewBlacklistUsecase(immediateTransaction{}, repo)).RegisterRoutes)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.auth {
				req.Header.Set("Authorization", adminPostRequest(t, "/").Header.Get("Authorization"))
			}
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if tc.status == 200 && res.Body.String() != "{\"total\":0,\"items\":[]}\n" {
				t.Fatal(res.Body.String())
			}
			if tc.status == 204 && (repo.owner != 1 || repo.target != 42) {
				t.Fatalf("owner=%d target=%d", repo.owner, repo.target)
			}
		})
	}
}

func TestBlacklistFullHTTPError(t *testing.T) {
	err := transportError(usecase.Conflict(usecase.CodeBlacklistFull, "黑名单最多允许 1000 人"))
	res := httptest.NewRecorder()
	httpx.EH(func(http.ResponseWriter, *http.Request) error { return err }).ServeHTTP(res, httptest.NewRequest("PUT", "/", nil))
	if res.Code != 409 {
		t.Fatalf("status=%d", res.Code)
	}
}
