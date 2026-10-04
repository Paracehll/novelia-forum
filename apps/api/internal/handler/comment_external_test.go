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
	"forum/internal/usecase"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type handlerSubjectResolver struct {
	valid    bool
	exists   bool
	checkErr error
	check    func(context.Context, string, string) bool
}

func (s handlerSubjectResolver) Type(kind string) (domain.CommentSubjectType, bool) {
	return domain.CommentSubjectNovel, kind == "novel"
}
func (s handlerSubjectResolver) Valid(kind, key string) bool {
	return kind == "novel" && key != "invalid" && s.valid
}
func (s handlerSubjectResolver) Check(ctx context.Context, kind, key string) (bool, error) {
	if s.check != nil {
		return s.check(ctx, kind, key), s.checkErr
	}
	return s.exists, s.checkErr
}

func TestExternalCommentCreationChecksSubject(t *testing.T) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": "member",
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		kind       string
		valid      bool
		exists     bool
		status     int
		wantChecks int
		checkErr   error
	}{
		{"exists", "novel", true, true, http.StatusCreated, 1, nil},
		{"missing", "novel", true, false, http.StatusNotFound, 1, nil},
		{"invalid", "novel", false, false, http.StatusBadRequest, 0, nil},
		{"unsupported", "unknown", false, false, http.StatusBadRequest, 0, nil},
		{
			"check failed", "novel", true, false,
			http.StatusInternalServerError, 1, errors.New("private upstream failure"),
		},
	} {
		for _, body := range []string{`{"content":"评论"}`, `{"content":"回复","rootId":7}`} {
			t.Run(tc.name+body, func(t *testing.T) {
				repo := &domainCommentRepository{}
				checks := 0
				checker := handlerSubjectResolver{valid: tc.valid, exists: tc.exists, checkErr: tc.checkErr}
				checker.check = func(ctx context.Context, kind, key string) bool {
					checks++
					if kind != tc.kind || key != "web-syosetu-n1234" {
						t.Errorf("got %s/%s", kind, key)
					}
					return tc.exists
				}
				router := chi.NewRouter()
				comments := usecase.NewCommentUsecase(immediateTransaction{}, repo, nil, nil, checker)
				NewExternalCommentHandler(comments).RegisterRoutes(router)
				req := httptest.NewRequest(
					http.MethodPost, "/"+tc.kind+"/web-syosetu-n1234", strings.NewReader(body),
				)
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				if res.Code != tc.status || checks != tc.wantChecks ||
					repo.written != (tc.status == http.StatusCreated) {
					t.Fatalf("status=%d checks=%d written=%v body=%s",
						res.Code, checks, repo.written, res.Body.String(),
					)
				}
				if tc.checkErr != nil && res.Body.String() != "服务器内部错误" {
					t.Fatalf("check failure response exposed details: %q", res.Body.String())
				}
			})
		}
	}
}

func TestExternalCommentReadsDoNotCheckSubject(t *testing.T) {
	router := chi.NewRouter()
	checker := handlerSubjectResolver{valid: true, check: func(context.Context, string, string) bool {
		t.Fatal("reading historical comments must not check subject")
		return false
	}}
	comments := usecase.NewCommentUsecase(immediateTransaction{}, &listingCommentRepository{rootID: 7}, nil, nil, checker)
	NewExternalCommentHandler(comments).RegisterRoutes(router)
	for _, path := range []string{"/novel/deleted", "/novel/deleted/7/reply"} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}
}
