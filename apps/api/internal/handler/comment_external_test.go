package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/subject"
	"forum/internal/usecase"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type subjectCheckFunc func(context.Context, string, string) error

func (f subjectCheckFunc) Check(ctx context.Context, kind, key string) (domain.CommentSubjectType, error) {
	return domain.CommentSubjectNovel, f(ctx, kind, key)
}

func TestExternalCommentCreationChecksSubject(t *testing.T) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": "member",
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"exists", nil, http.StatusCreated},
		{"missing", subject.ErrNotFound, http.StatusNotFound},
		{"invalid", subject.ErrInvalid, http.StatusBadRequest},
		{"unsupported", subject.ErrUnsupported, http.StatusBadRequest},
		{"unavailable", errors.New("upstream failed"), http.StatusInternalServerError},
	} {
		for _, body := range []string{`{"content":"评论"}`, `{"content":"回复","rootId":7}`} {
			t.Run(tc.name+body, func(t *testing.T) {
				repo := &domainCommentRepository{}
				checks := 0
				checker := subjectCheckFunc(func(ctx context.Context, kind, key string) error {
					checks++
					if kind != "novel" || key != "web-syosetu-n1234" {
						t.Errorf("got %s/%s", kind, key)
					}
					return tc.err
				})
				router := chi.NewRouter()
				comments := usecase.NewCommentUsecase(repo, nil, nil, checker)
				NewExternalCommentHandler(comments).RegisterRoutes(router)
				req := httptest.NewRequest(http.MethodPost, "/novel/web-syosetu-n1234", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				if tc.status == http.StatusInternalServerError && res.Body.String() != "服务器内部错误" {
					t.Fatalf("unexpected infrastructure error body: %q", res.Body.String())
				}
				if res.Code != tc.status || checks != 1 || repo.written != (tc.status == http.StatusCreated) {
					t.Fatalf("status=%d checks=%d written=%v body=%s", res.Code, checks, repo.written, res.Body.String())
				}
			})
		}
	}
}

func TestExternalCommentReadsDoNotCheckSubject(t *testing.T) {
	router := chi.NewRouter()
	checker := subjectCheckFunc(func(context.Context, string, string) error {
		t.Fatal("reading historical comments must not check subject")
		return nil
	})
	comments := usecase.NewCommentUsecase(&listingCommentRepository{rootID: 7}, nil, nil, checker)
	NewExternalCommentHandler(comments).RegisterRoutes(router)
	for _, path := range []string{"/novel/deleted", "/novel/deleted/7/reply"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}
}
