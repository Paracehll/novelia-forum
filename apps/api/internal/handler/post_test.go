package handler

import (
	"encoding/json"
	"fmt"
	"forum/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forum/internal/domain"
	"forum/internal/repository"
	"forum/internal/usecase"
)

type listPostRepository struct {
	repository.PostRepository
	items []domain.PostListItem
}

type capturingPostRepository struct {
	repository.PostRepository
	filter repository.PostFilter
}

func (r *capturingPostRepository) List(filter repository.PostFilter, _, _ int64) (int64, []domain.PostListItem, error) {
	r.filter = filter
	return 0, nil, nil
}

func TestAdminPostListStatusFilter(t *testing.T) {
	for _, tc := range []struct {
		query      string
		wantStatus domain.PostStatus
		wantError  bool
	}{
		{"", repository.PostStatusAll, false},
		{"?status=all", repository.PostStatusAll, false},
		{"?status=0", domain.PostStatusPublished, false},
		{"?status=1", domain.PostStatusHidden, false},
		{"?status=2", domain.PostStatusDeleted, false},
		{"?status=3", 0, true},
		{"?status=", 0, true},
		{"?status=1&status=2", 0, true},
	} {
		repo := &capturingPostRepository{}
		h := NewPostHandler(usecase.NewPostUsecase(repo, noFavoriteRepository{}, nil), nil)
		err := h.listAdminPosts(httptest.NewRecorder(), adminPostRequest(t, "/admin/post/"+tc.query))
		if (err != nil) != tc.wantError {
			t.Fatalf("query %q: error=%v, wantError=%v", tc.query, err, tc.wantError)
		}
		if tc.wantError {
			continue
		}
		if repo.filter.Status != tc.wantStatus {
			t.Fatalf("query %q: unexpected filter: %#v", tc.query, repo.filter)
		}
	}
}

func TestAdminPostListAuthorFilter(t *testing.T) {
	for _, tc := range []struct {
		query     string
		wantID    int64
		wantError bool
	}{
		{"", 0, false},
		{"?author_id=42", 42, false},
		{"?author_id=0", 0, true},
		{"?author_id=-1", 0, true},
		{"?author_id=abc", 0, true},
		{"?author_id=", 0, true},
		{"?author_id=1&author_id=2", 0, true},
	} {
		repo := &capturingPostRepository{}
		h := NewPostHandler(usecase.NewPostUsecase(repo, noFavoriteRepository{}, nil), nil)
		err := h.listAdminPosts(httptest.NewRecorder(), adminPostRequest(t, "/admin/post/"+tc.query))
		if (err != nil) != tc.wantError {
			t.Fatalf("query %q: error=%v, wantError=%v", tc.query, err, tc.wantError)
		}
		if !tc.wantError && repo.filter.AuthorID != tc.wantID {
			t.Fatalf("query %q: unexpected filter: %#v", tc.query, repo.filter)
		}
	}
}

func TestAdminPostListAuthorNameFilter(t *testing.T) {
	for _, tc := range []struct {
		query    string
		wantName string
	}{
		{"", ""},
		{"?author_name=alice", "alice"},
		{"?author_name=%20Alice%20", "Alice"},
		{"?author_name=小明", "小明"},
		{"?author_name=%20%20", ""},
		{"?author_name=alice&author_id=42&status=1", "alice"},
	} {
		repo := &capturingPostRepository{}
		h := NewPostHandler(usecase.NewPostUsecase(repo, noFavoriteRepository{}, nil), nil)
		err := h.listAdminPosts(httptest.NewRecorder(), adminPostRequest(t, "/admin/post/"+tc.query))
		if err != nil {
			t.Fatalf("query %q: %v", tc.query, err)
		}
		if repo.filter.AuthorName != tc.wantName {
			t.Fatalf("query %q: unexpected filter: %#v", tc.query, repo.filter)
		}
		if strings.Contains(tc.query, "author_id=42") && (repo.filter.AuthorID != 42 || repo.filter.Status != 1) {
			t.Fatalf("combined filters lost: %#v", repo.filter)
		}
	}
}

func TestValidatePostTextLimits(t *testing.T) {
	posts := usecase.NewPostUsecase(&writePostRepository{}, nil, nil)
	for _, tc := range []struct {
		name    string
		title   string
		content string
		wantErr bool
	}{
		{"minimum lengths", "标题", "文", false},
		{"maximum lengths", strings.Repeat("题", 100), strings.Repeat("文", 20000), false},
		{"short title", "题", "正文", true},
		{"long title", strings.Repeat("题", 101), "正文", true},
		{"blank title", " \n\t", "正文", true},
		{"empty content", "标题", "", true},
		{"blank content", "标题", " \n\t", true},
		{"long content", "标题", strings.Repeat("文", 20001), true},
		{"long content with padding", "标题", strings.Repeat("文", 20000) + " ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := posts.Create(usecase.Actor{UserID: 1}, usecase.PostInput{CategoryID: 1, Title: tc.title, Content: tc.content})
			if (err != nil) != tc.wantErr {
				t.Fatalf("validatePost() error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidatePostTagLimit(t *testing.T) {
	posts := usecase.NewPostUsecase(&writePostRepository{}, nil, nil)
	for _, tc := range []struct {
		name    string
		tagIDs  []int64
		wantErr bool
	}{
		{"no tags", nil, false},
		{"three tags", []int64{1, 2, 3}, false},
		{"four tags", []int64{1, 2, 3, 4}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := posts.Create(usecase.Actor{UserID: 1}, usecase.PostInput{CategoryID: 1, Title: "标题", Content: "正文", TagIDs: tc.tagIDs})
			if (err != nil) != tc.wantErr {
				t.Fatalf("validatePost() error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func (r listPostRepository) List(repository.PostFilter, int64, int64) (int64, []domain.PostListItem, error) {
	return int64(len(r.items)), r.items, nil
}

func TestPostListOmitsContentAndDetailPreservesIt(t *testing.T) {
	pinOrder := int32(1)
	createdAt := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	post := domain.Post{
		ID: 42, CategoryID: 1, Title: "标题", Content: "完整正文",
		AuthorID: 7, AuthorUsername: "alice", Status: domain.PostStatusPublished,
		ViewsCount: 10, CommentsCount: 3, CommentsLocked: true, PinOrder: &pinOrder,
		CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Hour), ActiveAt: createdAt.Add(2 * time.Hour),
		Tags: []domain.PostTag{{ID: 2, Name: "标签", Color: 3}},
	}
	listItem := domain.PostListItem{
		ID: post.ID, CategoryID: post.CategoryID, Title: post.Title,
		AuthorID: post.AuthorID, AuthorUsername: post.AuthorUsername, Status: post.Status,
		ViewsCount: post.ViewsCount, CommentsCount: post.CommentsCount,
		CommentsLocked: post.CommentsLocked, PinOrder: post.PinOrder,
		CreatedAt: post.CreatedAt, UpdatedAt: post.UpdatedAt, ActiveAt: post.ActiveAt,
		Tags: post.Tags,
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/post/", nil)
	if err := respondPosts(recorder, request, usecase.NewPostUsecase(listPostRepository{items: []domain.PostListItem{listItem}}, nil, nil).List, repository.PostFilter{}); err != nil {
		t.Fatal(err)
	}
	var response page[map[string]json.RawMessage]
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 1 || len(response.Items) != 1 {
		t.Fatalf("unexpected list: %s", recorder.Body.String())
	}
	item := response.Items[0]
	if _, ok := item["content"]; ok {
		t.Fatal("list must not include content, even as an empty string")
	}
	detailJSON, err := json.Marshal(newPostResponse(post, false))
	if err != nil {
		t.Fatal(err)
	}
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(detailJSON, &detail); err != nil {
		t.Fatal(err)
	}
	var content string
	if err := json.Unmarshal(detail["content"], &content); err != nil {
		t.Fatal(err)
	}
	if content != post.Content {
		t.Fatalf("detail content = %q", content)
	}
	delete(detail, "content")
	if len(item) != len(detail) {
		t.Fatal("list metadata fields differ from detail")
	}
	for key, value := range detail {
		if string(item[key]) != string(value) {
			t.Fatalf("list metadata %s differs from detail", key)
		}
	}
}

type writePostRepository struct {
	repository.PostRepository
	written bool
}

func (r *writePostRepository) Find(int64, bool) (*domain.Post, error) {
	return &domain.Post{ID: 42, AuthorID: 1}, nil
}

func (r *writePostRepository) Create(input repository.CreatePostInput) (*domain.Post, error) {
	r.written = true
	return &domain.Post{ID: 42, CategoryID: input.CategoryID}, nil
}

func (r *writePostRepository) Update(id int64, input repository.UpdatePostInput) (*domain.Post, error) {
	r.written = true
	return &domain.Post{ID: id, CategoryID: input.CategoryID}, nil
}

type noFavoriteRepository struct{ repository.FavoriteRepository }

func (noFavoriteRepository) Has(int64, int64) (bool, error) { return false, nil }
func (noFavoriteRepository) ListPostIDs(int64, []int64) (map[int64]bool, error) {
	return map[int64]bool{}, nil
}

func adminPostRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": "admin",
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	var authenticated *http.Request
	httpx.OptionalAccessToken(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		authenticated = r
	})).ServeHTTP(httptest.NewRecorder(), request)
	if authenticated == nil {
		t.Fatal("admin authentication failed")
	}
	return authenticated
}

func TestAnnouncementsPublishingRequiresAdmin(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		for _, role := range []string{"member", "trusted", "admin"} {
			for _, categoryID := range []int64{1, 2, 3} {
				t.Run(fmt.Sprintf("%s/%s/%d", method, role, categoryID), func(t *testing.T) {
					repo := &writePostRepository{}
					router := chi.NewRouter()
					NewPostHandler(usecase.NewPostUsecase(repo, noFavoriteRepository{}, nil), nil).RegisterRoutes(router)
					path := "/"
					wantStatus := http.StatusCreated
					if method == http.MethodPatch {
						path = "/42/"
						wantStatus = http.StatusOK
					}
					allowed := categoryID != 2 || role == "admin"
					if !allowed {
						wantStatus = http.StatusForbidden
					}
					request := httptest.NewRequest(method, path, strings.NewReader(fmt.Sprintf(
						`{"categoryId":%d,"title":"标题","content":"正文","tagIds":[]}`, categoryID)))
					request.Header.Set("Content-Type", "application/json")
					token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
						"sub": "tester", "uid": 1, "role": role,
					}).SignedString([]byte(httpx.AccessTokenSecret))
					if err != nil {
						t.Fatal(err)
					}
					request.Header.Set("Authorization", "Bearer "+token)
					recorder := httptest.NewRecorder()
					router.ServeHTTP(recorder, request)
					if recorder.Code != wantStatus || repo.written != allowed {
						t.Fatalf("status=%d want=%d written=%v allowed=%v body=%s", recorder.Code, wantStatus, repo.written, allowed, recorder.Body.String())
					}
				})
			}
		}
	}
}

type deletePostRepository struct {
	repository.PostRepository
	post    domain.Post
	deleted bool
}

func (r *deletePostRepository) Find(int64, bool) (*domain.Post, error) {
	return &r.post, nil
}

func (r *deletePostRepository) SetStatus(id int64, status domain.PostStatus) error {
	r.deleted = true
	if id != r.post.ID || status != domain.PostStatusDeleted {
		return fmt.Errorf("unexpected post status update: id=%d status=%d", id, status)
	}
	return nil
}

func TestPostDeletionWindow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		role       string
		userID     int64
		age        time.Duration
		wantStatus int
	}{
		{"author within window", "member", 1, 19 * time.Minute, http.StatusNoContent},
		{"author after window", "member", 1, 21 * time.Minute, http.StatusForbidden},
		{"admin after window", "admin", 2, 21 * time.Minute, http.StatusNoContent},
		{"other user within window", "member", 2, 19 * time.Minute, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &deletePostRepository{post: domain.Post{
				ID: 42, AuthorID: 1, CreatedAt: time.Now().Add(-tc.age),
			}}
			router := chi.NewRouter()
			NewPostHandler(usecase.NewPostUsecase(repo, noFavoriteRepository{}, nil), nil).RegisterRoutes(router)
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": "tester", "uid": tc.userID, "role": tc.role,
			}).SignedString([]byte(httpx.AccessTokenSecret))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodDelete, "/42/", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus || repo.deleted != (tc.wantStatus == http.StatusNoContent) {
				t.Fatalf("status=%d want=%d deleted=%v body=%s", recorder.Code, tc.wantStatus, repo.deleted, recorder.Body.String())
			}
		})
	}
}
