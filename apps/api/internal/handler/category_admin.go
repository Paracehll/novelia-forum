package handler

import (
	"net/http"
	"strings"
	"time"

	forumcategory "forum/internal/category"
	"forum/internal/httpx"
	"forum/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

func (h *categoryHandler) RegisterAdminRoutes(router chi.Router) {
	router.Get("/{cid}/tag", httpx.EH(h.listTags))
	router.Post("/{cid}/tag", httpx.EH(h.createTag))
	router.Put("/{cid}/tag/{id}", httpx.EH(h.updateTag))
	router.Put("/{cid}/tag/{id}/active", httpx.EH(h.activateTag))
	router.Delete("/{cid}/tag/{id}/active", httpx.EH(h.deactivateTag))
}

type tagResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     int16     `json:"color"`
	IsActive  bool      `json:"isActive"`
	SortOrder int32     `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (h *categoryHandler) listTags(w http.ResponseWriter, r *http.Request) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	if _, ok := forumcategory.FindByID(categoryID); !ok {
		return httpx.NotFound("分类不存在")
	}
	items, err := h.tagRepo.ListByCategory(categoryID)
	if err != nil {
		return httpx.InternalError(err, "查询标签失败")
	}
	response := make([]tagResponse, len(items))
	for i, item := range items {
		response[i] = tagResponse{
			ID:        item.ID,
			Name:      item.Name,
			Color:     item.Color,
			IsActive:  item.IsActive,
			SortOrder: item.SortOrder,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		}
	}
	render.JSON(w, r, response)
	return nil
}

type tagInput struct {
	Name      string `json:"name" validate:"required,max=64"`
	Color     int16  `json:"color" validate:"gte=0"`
	SortOrder int32  `json:"sortOrder"`
}

func (h *categoryHandler) createTag(w http.ResponseWriter, r *http.Request) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	if _, ok := forumcategory.FindByID(categoryID); !ok {
		return httpx.NotFound("分类不存在")
	}

	input, err := httpx.Body[tagInput](r)
	if err != nil {
		return err
	}

	tag, err := h.tagRepo.Create(
		categoryID,
		strings.TrimSpace(input.Name),
		input.Color,
		input.SortOrder,
		"{}",
	)
	if repository.IsUniqueViolation(err) {
		return httpx.Conflict("标签已存在")
	} else if err != nil {
		return httpx.InternalError(err, "创建标签失败")
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, tagResponse{
		ID:        tag.ID,
		Name:      tag.Name,
		Color:     tag.Color,
		IsActive:  tag.IsActive,
		SortOrder: tag.SortOrder,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	})
	return nil
}

func (h *categoryHandler) updateTag(w http.ResponseWriter, r *http.Request) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	if _, ok := forumcategory.FindByID(categoryID); !ok {
		return httpx.NotFound("分类不存在")
	}
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	input, err := httpx.Body[tagInput](r)
	if err != nil {
		return err
	}

	tag, err := h.tagRepo.Update(
		categoryID,
		id,
		strings.TrimSpace(input.Name),
		input.Color,
		input.SortOrder,
	)
	if repository.IsNotFound(err) {
		return httpx.NotFound("标签不存在")
	} else if err != nil {
		return httpx.InternalError(err, "更新标签失败")
	}

	render.JSON(w, r, tagResponse{
		ID:        tag.ID,
		Name:      tag.Name,
		Color:     tag.Color,
		IsActive:  tag.IsActive,
		SortOrder: tag.SortOrder,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	})
	return nil
}

func (h *categoryHandler) activateTag(w http.ResponseWriter, r *http.Request) error {
	return h.setTagActive(w, r, true)
}

func (h *categoryHandler) deactivateTag(w http.ResponseWriter, r *http.Request) error {
	return h.setTagActive(w, r, false)
}

func (h *categoryHandler) setTagActive(w http.ResponseWriter, r *http.Request, active bool) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	if _, ok := forumcategory.FindByID(categoryID); !ok {
		return httpx.NotFound("分类不存在")
	}
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	if err := h.tagRepo.SetActive(categoryID, id, active); repository.IsNotFound(err) {
		return httpx.NotFound("标签不存在")
	} else if err != nil {
		return httpx.InternalError(err, "设置标签启用状态失败")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
