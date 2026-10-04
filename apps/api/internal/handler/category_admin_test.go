package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"

	"github.com/golang-jwt/jwt/v5"

	"github.com/go-chi/chi/v5"
)

type capturingTagRepository struct {
	repository.TagRepository
	categoryID int64
	tagID      int64
	active     bool
	name       string
	err        error
}

func (r *capturingTagRepository) Update(categoryID, id int64, name string, color int16, sortOrder int32) (*repository.Tag, error) {
	r.categoryID, r.tagID = categoryID, id
	r.name = name
	return &repository.Tag{
		ID:         id,
		CategoryID: categoryID,
		Name:       name,
		Color:      color,
		SortOrder:  sortOrder,
	}, r.err
}

func (r *capturingTagRepository) SetActive(categoryID, id int64, active bool) error {
	r.categoryID, r.tagID, r.active = categoryID, id, active
	return r.err
}

func (r *capturingTagRepository) Create(cid int64, name string, color int16, order int32, attr string) (*repository.Tag, error) {
	return r.Update(cid, 9, name, color, order)
}

func (r *capturingTagRepository) ListByCategory(cid int64) ([]repository.Tag, error) {
	r.categoryID = cid
	return nil, r.err
}

func (r *capturingTagRepository) ListActive() ([]repository.Tag, error) {
	return []repository.Tag{{ID: 9, CategoryID: 1, Name: "标签", IsActive: true}}, r.err
}

func tagAdminToken(t *testing.T, role string) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "tester", "uid": 1, "role": role,
	}).SignedString([]byte(httpx.AccessTokenSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAdminTagMutationUsesCategoryID(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		wantActive bool
	}{
		{name: "update", method: http.MethodPut, path: "/1/tag/9", body: `{"name":"标签","color":2,"sortOrder":3}`},
		{name: "activate", method: http.MethodPut, path: "/1/tag/9/active", wantActive: true},
		{name: "deactivate", method: http.MethodDelete, path: "/1/tag/9/active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &capturingTagRepository{}
			router := chi.NewRouter()
			router.Use(httpx.OptionalAccessToken)
			NewCategoryHandler(usecase.NewTagUsecase(repo)).RegisterAdminRoutes(router)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			request.Header.Set("Authorization", "Bearer "+tagAdminToken(t, "admin"))
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code < 200 || response.Code >= 300 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if repo.categoryID != 1 || repo.tagID != 9 || repo.active != tc.wantActive {
				t.Fatalf("unexpected repository call: categoryID=%d tagID=%d active=%v",
					repo.categoryID, repo.tagID, repo.active,
				)
			}
		})
	}
}

func TestTagHandlers(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, role string
		repoErr                        error
		status                         int
	}{
		{name: "public list", method: "GET", path: "/", status: 200},
		{name: "empty admin list", method: "GET", path: "/1/tag", role: "admin", status: 200},
		{
			name: "create", method: "POST", path: "/1/tag",
			body: `{"name":" 标签 ","color":2}`, role: "admin", status: 201,
		},
		{
			name: "blank name", method: "POST", path: "/1/tag",
			body: `{"name":"　 "}`, role: "admin", status: 400,
		},
		{
			name: "negative color", method: "PUT", path: "/1/tag/9",
			body: `{"name":"标签","color":-1}`, role: "admin", status: 400,
		},
		{
			name: "create conflict", method: "POST", path: "/1/tag", body: `{"name":"标签"}`,
			role: "admin", repoErr: repository.ErrConflict, status: 409,
		},
		{
			name: "update conflict", method: "PUT", path: "/1/tag/9", body: `{"name":"标签"}`,
			role: "admin", repoErr: repository.ErrConflict, status: 409,
		},
		{
			name: "missing tag", method: "DELETE", path: "/1/tag/9/active",
			role: "admin", repoErr: repository.ErrNotFound, status: 404,
		},
		{
			name: "database failure", method: "GET", path: "/1/tag",
			role: "admin", repoErr: errors.New("private database details"), status: 500,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &capturingTagRepository{err: tc.repoErr}
			router := chi.NewRouter()
			router.Use(httpx.OptionalAccessToken)
			h := NewCategoryHandler(usecase.NewTagUsecase(repo))
			h.RegisterRoutes(router)
			h.RegisterAdminRoutes(router)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if tc.role != "" {
				request.Header.Set("Authorization", "Bearer "+tagAdminToken(t, tc.role))
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
			if tc.name == "create" && repo.name != "标签" {
				t.Fatalf("name=%q", repo.name)
			}
			if tc.name == "empty admin list" && strings.TrimSpace(response.Body.String()) != "[]" {
				t.Fatal(response.Body.String())
			}
			if tc.name == "public list" {
				var items []categoryListResponse
				if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil ||
					len(items) != 3 || len(items[0].Tags) != 1 || len(items[1].Tags) != 0 {
					t.Fatalf("public response=%s err=%v", response.Body.String(), err)
				}
			}
			if tc.status == 500 && strings.Contains(response.Body.String(), "private database") {
				t.Fatal("infrastructure details leaked")
			}
		})
	}
}

func TestTagAdminHandlersPassActor(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/1/tag", ""},
		{"POST", "/1/tag", `{"name":"标签"}`},
		{"PUT", "/1/tag/9", `{"name":"标签"}`},
		{"PUT", "/1/tag/9/active", ""},
		{"DELETE", "/1/tag/9/active", ""},
	} {
		for _, role := range []string{"", "member"} {
			t.Run(tc.method+tc.path+role, func(t *testing.T) {
				repo := &capturingTagRepository{}
				router := chi.NewRouter()
				router.Use(httpx.OptionalAccessToken)
				NewCategoryHandler(usecase.NewTagUsecase(repo)).RegisterAdminRoutes(router)
				request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				if tc.body != "" {
					request.Header.Set("Content-Type", "application/json")
				}
				if role != "" {
					request.Header.Set("Authorization", "Bearer "+tagAdminToken(t, role))
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusForbidden || repo.categoryID != 0 {
					t.Fatalf("status=%d category=%d body=%s",
						response.Code, repo.categoryID, response.Body.String(),
					)
				}
			})
		}
	}
}

func TestAdminTagMutationRejectsUnknownCategory(t *testing.T) {
	repo := &capturingTagRepository{}
	router := chi.NewRouter()
	router.Use(httpx.OptionalAccessToken)
	NewCategoryHandler(usecase.NewTagUsecase(repo)).RegisterAdminRoutes(router)
	request := httptest.NewRequest(http.MethodPut, "/999/tag/9/active", nil)
	request.Header.Set("Authorization", "Bearer "+tagAdminToken(t, "admin"))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if repo.categoryID != 0 || repo.tagID != 0 {
		t.Fatal("unknown category reached repository")
	}
}
