package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type listingCommentRepository struct {
	repository.CommentRepository
	rootID int64
}

func (r *listingCommentRepository) ListRoots(
	ctx context.Context,
	_ domain.CommentSubjectType,
	subjectKey string,
	limit, offset int64,
) (int64, []domain.CommentThreadPreview, error) {
	return 1, []domain.CommentThreadPreview{{
		Root:       domain.Comment{ID: r.rootID, SubjectKey: subjectKey, Content: "一级评论"},
		ReplyCount: 2,
	}}, nil
}

func (r *listingCommentRepository) ListReplies(
	ctx context.Context,
	_ domain.CommentSubjectType,
	subjectKey string,
	rootID, limit, offset int64,
) (int64, []domain.Comment, error) {
	return 2, []domain.Comment{{
		ID: 9, SubjectKey: subjectKey, RootID: &rootID, Content: "二级评论",
	}}, nil
}

func TestExternalCommentRepliesArePaginatedSeparately(t *testing.T) {
	repo := &listingCommentRepository{rootID: 7}
	router := chi.NewRouter()
	resolver := handlerSubjectResolver{valid: true, exists: true}
	NewExternalCommentHandler(
		usecase.NewCommentUsecase(immediateTransaction{}, repo, &writePostRepository{}, nil, resolver),
	).RegisterRoutes(router)

	for _, tc := range []struct {
		path           string
		wantTotal      int64
		wantID         int64
		wantRootID     *int64
		wantReplyCount int64
	}{
		{path: "/novel/chapter-1?page=1&page_size=10", wantTotal: 1, wantID: 7, wantReplyCount: 2},
		{path: "/novel/chapter-1/7/reply?page=1&page_size=10", wantTotal: 2, wantID: 9, wantRootID: &repo.rootID},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s: status=%d body=%s", tc.path, recorder.Code, recorder.Body.String())
		}
		var response page[externalCommentResponse]
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Total != tc.wantTotal || len(response.Items) != 1 ||
			response.Items[0].ID != tc.wantID || response.Items[0].ReplyCount != tc.wantReplyCount {
			t.Fatalf("GET %s: unexpected response %#v", tc.path, response)
		}
		if tc.wantRootID == nil && response.Items[0].RootID != nil {
			t.Fatalf("GET %s: expected a root comment", tc.path)
		}
		if tc.wantRootID != nil &&
			(response.Items[0].RootID == nil || *response.Items[0].RootID != *tc.wantRootID) {
			t.Fatalf("GET %s: unexpected root ID", tc.path)
		}
	}
}

type editableCommentRepository struct {
	repository.CommentRepository
	comment domain.Comment
	updated bool
	deleted bool
}

func (r *editableCommentRepository) Find(
	context.Context,
	domain.CommentSubjectType,
	int64,
) (*domain.Comment, error) {
	return &r.comment, nil
}

func (r *editableCommentRepository) Lock(_ context.Context, kind domain.CommentSubjectType, _ int64) (*domain.Comment, error) {
	copy := r.comment
	copy.SubjectType = kind
	if copy.SubjectKey == "" {
		copy.SubjectKey = "42"
	}
	return &copy, nil
}

func (r *editableCommentRepository) Update(
	ctx context.Context,
	_ domain.CommentSubjectType,
	_ int64,
	content string,
) (*domain.Comment, error) {
	r.updated = true
	r.comment.Content = content
	return &r.comment, nil
}

func (r *editableCommentRepository) SetStatus(
	ctx context.Context,
	_ domain.CommentSubjectType,
	_ int64,
	status domain.CommentStatus,
) error {
	r.deleted = status == domain.CommentStatusDeleted
	return nil
}

func TestCommentEditingStateResponses(t *testing.T) {
	for _, route := range []struct {
		name, path string
		external   bool
	}{
		{"post", "/7", false},
		{"external", "/novel/7", true},
	} {
		for _, role := range []string{"member", "admin"} {
			for _, status := range []domain.CommentStatus{
				domain.CommentStatusPublished,
				domain.CommentStatusHidden,
				domain.CommentStatusDeleted,
				99,
			} {
				t.Run(route.name+"/"+role+"/"+strconv.Itoa(int(status)), func(t *testing.T) {
					repo := &editableCommentRepository{comment: domain.Comment{
						ID: 7, SubjectKey: "42", AuthorID: 1,
						Content: "original", Status: status, CreatedAt: time.Now(),
					}}
					u := usecase.NewCommentUsecase(immediateTransaction{},
						repo, &writePostRepository{}, nil, handlerSubjectResolver{valid: true, exists: true},
					)
					router := chi.NewRouter()
					if route.external {
						NewExternalCommentHandler(u).RegisterRoutes(router)
					} else {
						NewCommentHandler(u).RegisterRoutes(router)
					}
					token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
						"sub": "tester", "uid": 1, "role": role,
					}).SignedString([]byte(httpx.AccessTokenSecret))
					if err != nil {
						t.Fatal(err)
					}
					request := httptest.NewRequest(
						http.MethodPatch, route.path, strings.NewReader(`{"content":"updated"}`),
					)
					request.Header.Set("Authorization", "Bearer "+token)
					request.Header.Set("Content-Type", "application/json")
					recorder := httptest.NewRecorder()
					router.ServeHTTP(recorder, request)
					wantStatus := http.StatusConflict
					if status == domain.CommentStatusPublished {
						wantStatus = http.StatusOK
					}
					if recorder.Code != wantStatus || repo.updated != (status == domain.CommentStatusPublished) {
						t.Fatalf("status=%d want=%d updated=%t body=%s",
							recorder.Code, wantStatus, repo.updated, recorder.Body.String(),
						)
					}
					if wantStatus == http.StatusConflict && recorder.Body.String() != "只有已发布的评论可以编辑" {
						t.Fatalf("unexpected state error: %q", recorder.Body.String())
					}
				})
			}
		}
	}
}

func TestAdminCanEditCommentAfterWindow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		role       string
		userID     int64
		age        time.Duration
		wantStatus int
	}{
		{"author within window", "member", 1, 19 * time.Minute, http.StatusOK},
		{"author after window", "member", 1, 21 * time.Minute, http.StatusForbidden},
		{"admin after window", "admin", 2, 21 * time.Minute, http.StatusOK},
		{"admin editing own old comment", "admin", 1, 21 * time.Minute, http.StatusOK},
		{"other member within window", "member", 2, 19 * time.Minute, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &editableCommentRepository{comment: domain.Comment{
				ID: 7, SubjectKey: "42", AuthorID: 1, CreatedAt: time.Now().Add(-tc.age),
			}}
			router := chi.NewRouter()
			NewCommentHandler(usecase.NewCommentUsecase(immediateTransaction{}, repo, &writePostRepository{}, nil, nil)).RegisterRoutes(router)
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": "tester", "uid": tc.userID, "role": tc.role,
			}).SignedString([]byte(httpx.AccessTokenSecret))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPatch, "/7", strings.NewReader(`{"content":"更新内容"}`))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus || repo.updated != (tc.wantStatus == http.StatusOK) {
				t.Fatalf("status=%d want=%d updated=%v body=%s",
					recorder.Code, tc.wantStatus, repo.updated, recorder.Body.String(),
				)
			}
		})
	}
}

func TestAdminCanModifyExternalCommentAfterWindow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		role       string
		wantStatus int
	}{
		{"member cannot edit", http.MethodPatch, "member", http.StatusForbidden},
		{"admin can edit", http.MethodPatch, "admin", http.StatusOK},
		{"member cannot delete", http.MethodDelete, "member", http.StatusForbidden},
		{"admin can delete", http.MethodDelete, "admin", http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &editableCommentRepository{comment: domain.Comment{
				ID: 7, SubjectType: domain.CommentSubjectNovel, SubjectKey: "chapter-1",
				AuthorID: 1, CreatedAt: time.Now().Add(-21 * time.Minute),
			}}
			router := chi.NewRouter()
			resolver := handlerSubjectResolver{valid: true, exists: true}
			NewExternalCommentHandler(
				usecase.NewCommentUsecase(immediateTransaction{}, repo, &writePostRepository{}, nil, resolver),
			).RegisterRoutes(router)
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": "tester", "uid": 1, "role": tc.role,
			}).SignedString([]byte(httpx.AccessTokenSecret))
			if err != nil {
				t.Fatal(err)
			}
			body := ""
			if tc.method == http.MethodPatch {
				body = `{"content":"更新内容"}`
			}
			request := httptest.NewRequest(tc.method, "/novel/7", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+token)
			if body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			wantModified := tc.role == "admin"
			if tc.method == http.MethodPatch && repo.updated != wantModified {
				t.Fatalf("updated=%v want=%v", repo.updated, wantModified)
			}
			if tc.method == http.MethodDelete && repo.deleted != wantModified {
				t.Fatalf("deleted=%v want=%v", repo.deleted, wantModified)
			}
		})
	}
}

func TestValidateCommentUpdate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"content only", `{"content":"更新内容"}`, false},
		{"root ID", `{"content":"更新内容","rootId":1}`, true},
		{"null root ID", `{"content":"更新内容","rootId":null}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPatch, "/comment/1", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			input, err := httpx.Body[commentUpdateInput](request)
			if err == nil {
				err = validateCommentUpdate(input)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("comment update error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestCommentResponsesPreserveContent(t *testing.T) {
	rootID := int64(1)
	comment := domain.Comment{
		ID: 2, SubjectKey: "123", RootID: &rootID,
		Content: "原始内容", Status: domain.CommentStatusHidden,
	}
	post, err := newCommentResponse(comment)
	if err != nil {
		t.Fatal(err)
	}
	external := newExternalCommentResponse(comment)
	if post.Content != comment.Content || external.Content != comment.Content ||
		post.Status != int16(comment.Status) || external.Status != int16(comment.Status) ||
		post.ID != comment.ID || external.ID != comment.ID ||
		post.RootID == nil || external.RootID == nil ||
		*post.RootID != rootID || *external.RootID != rootID {
		t.Fatal("comment response must preserve usecase result")
	}
}

func TestEmbeddedReplyResponse(t *testing.T) {
	rootID := int64(1)
	thread := domain.CommentThreadPreview{
		Root:       domain.Comment{ID: rootID, SubjectKey: domain.PostCommentSubjectKey(42)},
		ReplyCount: 3,
	}
	for _, status := range []domain.CommentStatus{
		domain.CommentStatusPublished,
		domain.CommentStatusHidden,
		domain.CommentStatusDeleted,
	} {
		thread.Replies = append(thread.Replies, domain.Comment{
			ID: int64(status) + 2, SubjectKey: thread.Root.SubjectKey, RootID: &rootID,
			Content: "body", Status: status,
		})
	}
	post, err := newCommentThreadResponse(thread)
	if err != nil {
		t.Fatal(err)
	}
	external := newExternalCommentThreadResponse(thread)
	if post.Replies == nil || external.Replies == nil ||
		post.Replies.Total != 3 || external.Replies.Total != 3 {
		t.Fatal("missing embedded reply page")
	}
	for i, expected := range []string{"body", "body", "body"} {
		if post.Replies.Items[i].Content != expected || external.Replies.Items[i].Content != expected {
			t.Fatalf("reply %d content was not preserved", i)
		}
	}
	thread.Replies = nil
	thread.ReplyCount = 0
	empty, err := newCommentThreadResponse(thread)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Replies == nil || empty.Replies.Items == nil ||
		len(empty.Replies.Items) != 0 || empty.Replies.Total != 0 {
		t.Fatal("empty reply page must use an empty array")
	}
}

func TestValidateComment(t *testing.T) {
	zero := int64(0)
	positive := int64(1)
	for _, tc := range []struct {
		name    string
		input   commentInput
		wantErr bool
	}{
		{"normal", commentInput{Content: "评论", RootID: &positive}, false},
		{"invalid root", commentInput{Content: "评论", RootID: &zero}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateComment(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateComment() error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestInvalidCommentRequestsStopBeforeUsecase(t *testing.T) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": "admin",
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	// Nil usecases make accidental calls past handler-only validation fail. The
	// external routes use a real usecase because subject rules live there.
	router := chi.NewRouter()
	router.Route("/post", NewPostHandler(nil, nil).RegisterRoutes)
	router.Route("/comment", NewCommentHandler(nil).RegisterRoutes)
	router.Route("/admin/comment", NewCommentHandler(nil).RegisterAdminRoutes)
	externalComments := usecase.NewCommentUsecase(immediateTransaction{},
		&domainCommentRepository{}, nil, nil,
		handlerSubjectResolver{valid: true, exists: true},
	)
	router.Route("/external", NewExternalCommentHandler(externalComments).RegisterRoutes)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/post/42/comment", `{"content":"评论","rootId":0}`},
		{http.MethodPut, "/admin/comment/7/status", `{"status":"invalid"}`},
		{http.MethodPost, "/external/novel/wenku-book", `{"content":"评论","rootId":-1}`},
		{http.MethodPost, "/external/unknown/wenku-book", `{"content":"评论"}`},
		{http.MethodPost, "/external/novel/invalid", `{"content":"评论"}`},
		{http.MethodPut, "/external/novel/7/status", `{"status":"invalid"}`},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestInvalidCommentContentResponses(t *testing.T) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": "member",
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/post/42/comment"},
		{http.MethodPatch, "/comment/7"},
		{http.MethodPost, "/external/novel/wenku-book"},
		{http.MethodPatch, "/external/novel/7"},
	} {
		for _, content := range []string{"", " \n\t", strings.Repeat("字", 1001)} {
			t.Run(route.method+route.path+"/"+strconv.Itoa(len(content)), func(t *testing.T) {
				repo := &domainCommentRepository{}
				resolver := handlerSubjectResolver{valid: true, exists: true}
				comments := usecase.NewCommentUsecase(immediateTransaction{}, repo, &writePostRepository{}, nil, resolver)
				router := chi.NewRouter()
				router.Route("/post", NewPostHandler(nil, comments).RegisterRoutes)
				router.Route("/comment", NewCommentHandler(comments).RegisterRoutes)
				router.Route("/external", NewExternalCommentHandler(comments).RegisterRoutes)
				body, err := json.Marshal(map[string]string{"content": content})
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(string(body)))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+token)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusBadRequest ||
					response.Body.String() != "评论内容不能为空且不能超过 1000 字" || repo.written {
					t.Fatalf("status=%d body=%q written=%v", response.Code, response.Body.String(), repo.written)
				}
			})
		}
	}
}
