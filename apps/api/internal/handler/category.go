package handler

import (
	"net/http"

	forumcategory "forum/internal/category"
	"forum/internal/httpx"
	"forum/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type categoryHandler struct {
	tagRepo repository.TagRepository
}

func NewCategoryHandler(tagRepo repository.TagRepository) *categoryHandler {
	return &categoryHandler{tagRepo: tagRepo}
}

func (h *categoryHandler) RegisterRoutes(router chi.Router) {
	router.Get("/", httpx.EH(h.list))
}

type categoryListResponse struct {
	ID   int64                 `json:"id"`
	Slug string                `json:"slug"`
	Tags []categoryTagResponse `json:"tags"`
}

type categoryTagResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     int16  `json:"color"`
	SortOrder int32  `json:"sortOrder"`
}

func (h *categoryHandler) list(w http.ResponseWriter, r *http.Request) error {
	tags, err := h.tagRepo.ListActive()
	if err != nil {
		return httpx.InternalError(err, "查询标签失败")
	}
	render.JSON(w, r, newCategoryResponses(forumcategory.List(), tags))
	return nil
}

func newCategoryResponses(items []forumcategory.Definition, tags []repository.Tag) []categoryListResponse {
	tagsByCategory := make(map[int64][]categoryTagResponse, len(items))
	for _, tag := range tags {
		tagsByCategory[tag.CategoryID] = append(tagsByCategory[tag.CategoryID], categoryTagResponse{
			ID:        tag.ID,
			Name:      tag.Name,
			Color:     tag.Color,
			SortOrder: tag.SortOrder,
		})
	}
	response := make([]categoryListResponse, len(items))
	for i, item := range items {
		categoryTags := tagsByCategory[item.ID]
		if categoryTags == nil {
			categoryTags = []categoryTagResponse{}
		}
		response[i] = categoryListResponse{
			ID:   item.ID,
			Slug: item.Slug,
			Tags: categoryTags,
		}
	}
	return response
}
