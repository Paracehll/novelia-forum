package handler

import (
	"net/http"
	"time"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/usecase"

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

func newTagResponse(tag domain.Tag) tagResponse {
	return tagResponse{ID: tag.ID, Name: tag.Name, Color: tag.Color, IsActive: tag.IsActive,
		SortOrder: tag.SortOrder, CreatedAt: tag.CreatedAt, UpdatedAt: tag.UpdatedAt}
}

func (h *categoryHandler) listTags(w http.ResponseWriter, r *http.Request) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	items, err := h.tagUsecase.ListAdmin(commentActor(principal), usecase.ListTagsQuery{CategoryID: categoryID})
	if err != nil {
		return transportError(err)
	}
	response := make([]tagResponse, len(items))
	for i, item := range items {
		response[i] = newTagResponse(item)
	}
	render.JSON(w, r, response)
	return nil
}

type tagInput struct {
	Name      string `json:"name"`
	Color     int16  `json:"color"`
	SortOrder int32  `json:"sortOrder"`
}

func (input tagInput) command(categoryID int64) usecase.TagInput {
	return usecase.TagInput{CategoryID: categoryID, Name: input.Name, Color: input.Color, SortOrder: input.SortOrder}
}

func (h *categoryHandler) createTag(w http.ResponseWriter, r *http.Request) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	input, err := httpx.Body[tagInput](r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	tag, err := h.tagUsecase.Create(commentActor(principal), input.command(categoryID))
	if err != nil {
		return transportError(err)
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, newTagResponse(*tag))
	return nil
}

func (h *categoryHandler) updateTag(w http.ResponseWriter, r *http.Request) error {
	categoryID, err := httpx.ParseParamPositiveInt(r, "cid")
	if err != nil {
		return err
	}
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	input, err := httpx.Body[tagInput](r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	tag, err := h.tagUsecase.Update(commentActor(principal), id, input.command(categoryID))
	if err != nil {
		return transportError(err)
	}
	render.JSON(w, r, newTagResponse(*tag))
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
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.tagUsecase.SetActive(commentActor(principal), usecase.SetTagActiveCommand{CategoryID: categoryID, ID: id, Active: active}); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
