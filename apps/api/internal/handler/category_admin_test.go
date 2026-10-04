package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forum/internal/repository"

	"github.com/go-chi/chi/v5"
)

type capturingTagRepository struct {
	repository.TagRepository
	categoryID int64
	tagID      int64
	active     bool
}

func (r *capturingTagRepository) Update(categoryID, id int64, name string, color int16, sortOrder int32) (*repository.Tag, error) {
	r.categoryID, r.tagID = categoryID, id
	return &repository.Tag{ID: id, CategoryID: categoryID, Name: name, Color: color, SortOrder: sortOrder}, nil
}

func (r *capturingTagRepository) SetActive(categoryID, id int64, active bool) error {
	r.categoryID, r.tagID, r.active = categoryID, id, active
	return nil
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
			NewCategoryHandler(repo).RegisterAdminRoutes(router)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code < 200 || response.Code >= 300 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if repo.categoryID != 1 || repo.tagID != 9 || repo.active != tc.wantActive {
				t.Fatalf("unexpected repository call: categoryID=%d tagID=%d active=%v", repo.categoryID, repo.tagID, repo.active)
			}
		})
	}
}

func TestAdminTagMutationRejectsUnknownCategory(t *testing.T) {
	repo := &capturingTagRepository{}
	router := chi.NewRouter()
	NewCategoryHandler(repo).RegisterAdminRoutes(router)
	request := httptest.NewRequest(http.MethodPut, "/999/tag/9/active", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if repo.categoryID != 0 || repo.tagID != 0 {
		t.Fatal("unknown category reached repository")
	}
}
