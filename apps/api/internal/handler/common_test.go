package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forum/internal/domain"
	"forum/internal/domainfilter"
	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

func TestActorFromPrincipal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal httpx.Principal
		want      usecase.Actor
	}{
		{"anonymous", httpx.Principal{}, usecase.Actor{}},
		{
			"member",
			httpx.Principal{UserID: 7, Username: "alice", Role: "member"},
			usecase.Actor{UserID: 7, Username: "alice"},
		},
		{
			"admin",
			httpx.Principal{UserID: 8, Username: "bob", Role: "admin"},
			usecase.Actor{UserID: 8, Username: "bob", IsAdmin: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := actorFromPrincipal(tc.principal); got != tc.want {
				t.Fatalf("actor=%+v want=%+v", got, tc.want)
			}
		})
	}
}

type domainCommentRepository struct {
	repository.CommentRepository
	written bool
}

func (r *domainCommentRepository) Find(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	id int64,
) (*domain.Comment, error) {
	return &domain.Comment{ID: id, AuthorID: 1, SubjectType: subjectType, CreatedAt: time.Now()}, nil
}

func (r *domainCommentRepository) Create(context.Context, domain.Comment) (*domain.Comment, error) {
	r.written = true
	return &domain.Comment{}, nil
}

func (r *domainCommentRepository) Update(
	context.Context,
	domain.CommentSubjectType,
	int64,
	string,
) (*domain.Comment, error) {
	r.written = true
	return &domain.Comment{}, nil
}

func TestDomainFilterRejectsWrites(t *testing.T) {
	domains, err := domainfilter.New([]domainfilter.Rule{{Domain: "evil.example", IncludeSubdomains: true}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": "member",
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, method, path, body string
	}{
		{"post title create", http.MethodPost, "/post/", `{"categoryId":1,"title":"evil.example","content":"正文"}`},
		{"post content create", http.MethodPost, "/post/", `{"categoryId":1,"title":"标题","content":"evil.example"}`},
		{"post title update", http.MethodPatch, "/post/42/", `{"categoryId":1,"title":"evil.example","content":"正文"}`},
		{"post content update", http.MethodPatch, "/post/42/", `{"categoryId":1,"title":"标题","content":"evil.example"}`},
		{"post comment create", http.MethodPost, "/post/42/comment", `{"content":"evil.example"}`},
		{"post comment update", http.MethodPatch, "/comment/7", `{"content":"evil.example"}`},
		{"external comment create", http.MethodPost, "/external/comment/novel/wenku-book", `{"content":"evil.example"}`},
		{"external comment update", http.MethodPatch, "/external/comment/novel/7", `{"content":"evil.example"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			posts := &writePostRepository{}
			comments := &domainCommentRepository{}
			router := chi.NewRouter()
			resolver := handlerSubjectResolver{valid: true, exists: true}
			commentUsecase := usecase.NewCommentUsecase(immediateTransaction{}, comments, posts, domains, resolver)
			postHandler := NewPostHandler(
				usecase.NewPostUsecase(immediateTransaction{}, posts, noFavoriteRepository{}, domains), commentUsecase,
			)
			router.Route("/post", postHandler.RegisterRoutes)
			router.Route("/comment", NewCommentHandler(commentUsecase).RegisterRoutes)
			router.Route("/external/comment", NewExternalCommentHandler(commentUsecase).RegisterRoutes)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || posts.written || comments.written {
				t.Fatalf("status=%d postWritten=%v commentWritten=%v body=%s",
					response.Code, posts.written, comments.written, response.Body.String(),
				)
			}
			if !strings.Contains(response.Body.String(), "禁止使用的域名") {
				t.Fatalf("unexpected response: %s", response.Body.String())
			}
		})
	}
}

func TestDomainFilterErrorMapping(t *testing.T) {
	domains, err := domainfilter.New([]domainfilter.Rule{{Domain: "evil.example", IncludeSubdomains: true}})
	if err != nil {
		t.Fatal(err)
	}
	value := strings.Repeat("a", 4097) + ".example"
	for _, filter := range []*domainfilter.Filter{nil, domains} {
		posts := usecase.NewPostUsecase(immediateTransaction{}, &writePostRepository{}, nil, filter)
		response := httptest.NewRecorder()
		httpx.EH(func(http.ResponseWriter, *http.Request) error {
			_, err := posts.Create(context.Background(), usecase.Actor{UserID: 1}, usecase.PostInput{
				CategoryID: 1, Title: "标题", Content: value,
			})
			return transportError(err)
		})(response, httptest.NewRequest(http.MethodPost, "/", nil))
		if filter == nil {
			if response.Code != http.StatusOK {
				t.Fatalf("disabled filter rejected content: %s", response.Body.String())
			}
		} else if response.Code != http.StatusBadRequest || response.Body.String() != "content 无法完成域名检查" {
			t.Fatalf("unexpected response: %d %q", response.Code, response.Body.String())
		}
	}
}
