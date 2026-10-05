package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type adminCommentRepository struct {
	repository.CommentRepository
	filter        repository.CommentFilter
	limit, offset int64
	called        bool
}

func (r *adminCommentRepository) ListAdmin(
	ctx context.Context,
	filter repository.CommentFilter,
	limit, offset int64,
) (int64, []domain.Comment, error) {
	r.called = true
	r.filter, r.limit, r.offset = filter, limit, offset
	return 1, []domain.Comment{{
		ID:          5,
		SubjectKey:  "42",
		SubjectType: domain.CommentSubjectPost,
		Content:     "隐藏评论原文",
		Status:      domain.CommentStatusHidden,
	}}, nil
}

func (r *adminCommentRepository) SetStatus(
	context.Context,
	domain.CommentSubjectType,
	int64,
	domain.CommentStatus,
) error {
	r.called = true
	return nil
}

func (r *adminCommentRepository) Lock(_ context.Context, kind domain.CommentSubjectType, id int64) (*domain.Comment, error) {
	return &domain.Comment{ID: id, SubjectType: kind, SubjectKey: "42", Status: domain.CommentStatusPublished}, nil
}
func (r *adminCommentRepository) SetStatusByAuthor(context.Context, int64, []domain.CommentStatus, domain.CommentStatus) ([]domain.Comment, error) {
	r.called = true
	return nil, nil
}

func commentStatus(value domain.CommentStatus) *domain.CommentStatus { return &value }

func TestCommentAdminHandlersPassActor(t *testing.T) {
	for _, route := range []struct {
		name, method, path, body string
		wantStatus               int
	}{
		{"list", http.MethodGet, "/admin/comment/", "", http.StatusOK},
		{"set status", http.MethodPut, "/admin/comment/7/status", `{"status":"hidden"}`, http.StatusNoContent},
		{"delete by author", http.MethodDelete, "/admin/comment/author/1", "", http.StatusNoContent},
		{"set external status", http.MethodPut, "/external/comment/novel/7/status", `{"status":"hidden"}`, http.StatusNoContent},
	} {
		for _, role := range []string{"member", "admin"} {
			t.Run(route.name+"/"+role, func(t *testing.T) {
				repo := &adminCommentRepository{}
				u := usecase.NewCommentUsecase(immediateTransaction{},
					repo, &writePostRepository{}, nil, handlerSubjectResolver{valid: true, exists: true},
				)
				router := chi.NewRouter()
				router.Use(httpx.OptionalAccessToken)
				// Deliberately omit the admin group's middleware to exercise usecase authorization.
				router.Route("/admin/comment", NewCommentHandler(u).RegisterAdminRoutes)
				router.Route("/external/comment", NewExternalCommentHandler(u).RegisterRoutes)
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": "tester", "uid": 1, "role": role,
				}).SignedString([]byte(httpx.AccessTokenSecret))
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				request.Header.Set("Authorization", "Bearer "+token)
				if route.body != "" {
					request.Header.Set("Content-Type", "application/json")
				}
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				wantStatus := http.StatusForbidden
				if role == "admin" {
					wantStatus = route.wantStatus
				}
				if recorder.Code != wantStatus || repo.called != (role == "admin") {
					t.Fatalf("status=%d want=%d called=%t body=%s",
						recorder.Code, wantStatus, repo.called, recorder.Body.String(),
					)
				}
			})
		}
	}
}

func TestAdminCommentList(t *testing.T) {
	for _, tc := range []struct {
		name, role, query string
		wantStatus        int
		wantFilter        repository.CommentFilter
		limit, offset     int64
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "member", role: "member", wantStatus: http.StatusForbidden},
		{
			name: "all posts", role: "admin", wantStatus: http.StatusOK,
			wantFilter: repository.CommentFilter{}, limit: 20,
		},
		{
			name: "combined filters", role: "admin",
			query:      "?q=%20内容%20&author_name=%20小明%20&post_id=42&status=1&page=2&page_size=10",
			wantStatus: http.StatusOK,
			wantFilter: repository.CommentFilter{
				Search:     "内容",
				AuthorName: "小明",
				PostID:     42,
				Status:     commentStatus(domain.CommentStatusHidden),
			},
			limit: 10, offset: 10,
		},
		{
			name: "published", role: "admin", query: "?status=0", wantStatus: http.StatusOK,
			wantFilter: repository.CommentFilter{Status: commentStatus(domain.CommentStatusPublished)},
			limit:      20,
		},
		{
			name: "deleted", role: "admin", query: "?status=2", wantStatus: http.StatusOK,
			wantFilter: repository.CommentFilter{Status: commentStatus(domain.CommentStatusDeleted)},
			limit:      20,
		},
		{
			name: "explicit all", role: "admin", query: "?status=all", wantStatus: http.StatusOK,
			wantFilter: repository.CommentFilter{}, limit: 20,
		},
		{name: "bad post", role: "admin", query: "?post_id=0", wantStatus: http.StatusBadRequest},
		{name: "empty post", role: "admin", query: "?post_id=", wantStatus: http.StatusBadRequest},
		{name: "multiple posts", role: "admin", query: "?post_id=1&post_id=2", wantStatus: http.StatusBadRequest},
		{name: "bad status", role: "admin", query: "?status=3", wantStatus: http.StatusBadRequest},
		{name: "empty status", role: "admin", query: "?status=", wantStatus: http.StatusBadRequest},
		{name: "negative status", role: "admin", query: "?status=-1", wantStatus: http.StatusBadRequest},
		{name: "overflow status", role: "admin", query: "?status=65536", wantStatus: http.StatusBadRequest},
		{name: "multiple statuses", role: "admin", query: "?status=0&status=1", wantStatus: http.StatusBadRequest},
		{name: "bad pagination", role: "admin", query: "?page=0", wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &adminCommentRepository{}
			router := chi.NewRouter()
			router.Use(httpx.RequireAdmin)
			NewCommentHandler(usecase.NewCommentUsecase(immediateTransaction{}, repo, nil, nil, nil)).RegisterAdminRoutes(router)
			request := httptest.NewRequest(http.MethodGet, "/"+tc.query, nil)
			if tc.role != "" {
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": "tester", "uid": 1, "role": tc.role,
				}).SignedString([]byte(httpx.AccessTokenSecret))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+token)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if repo.called {
					t.Fatal("invalid or unauthorized request reached repository")
				}
				return
			}
			if !reflect.DeepEqual(repo.filter, tc.wantFilter) ||
				repo.limit != tc.limit || repo.offset != tc.offset {
				t.Fatalf("unexpected query: %#v limit=%d offset=%d", repo.filter, repo.limit, repo.offset)
			}
			var response page[commentResponse]
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Total != 1 || len(response.Items) != 1 ||
				response.Items[0].Content != "隐藏评论原文" || response.Items[0].PostID != 42 {
				t.Fatalf("unexpected response: %#v", response)
			}
		})
	}
}
