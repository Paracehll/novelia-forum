package handler

import (
	"net/http"
	"strconv"
	"strings"

	"forum/internal/domain"
	"forum/internal/httpx"

	"github.com/go-chi/chi/v5"
)

func (h *postHandler) RegisterAdminRoutes(router chi.Router) {
	router.Get("/", httpx.EH(h.listAdminPosts))
	router.Put("/{id}/status", httpx.EH(h.setPostStatus))
	router.Put("/{id}/lock", httpx.EH(h.lockPostComments))
	router.Delete("/{id}/lock", httpx.EH(h.unlockPostComments))
	router.Put("/{id}/pin", httpx.EH(h.pinPost))
	router.Delete("/{id}/pin", httpx.EH(h.unpinPost))
}

func (h *postHandler) listAdminPosts(w http.ResponseWriter, r *http.Request) error {
	listQuery, err := parsePostListQuery(r)
	if err != nil {
		return err
	}
	listQuery.AuthorName = strings.TrimSpace(r.URL.Query().Get("author_name"))
	if values, ok := r.URL.Query()["author_id"]; ok {
		if len(values) != 1 {
			return httpx.BadRequest("author_id 必须为正整数")
		}
		authorID, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || authorID <= 0 {
			return httpx.BadRequest("author_id 必须为正整数")
		}
		listQuery.AuthorID = authorID
	}
	if values, ok := r.URL.Query()["status"]; ok {
		if len(values) != 1 {
			return httpx.BadRequest("status 必须为 all、0、1 或 2")
		}
		if values[0] != "all" {
			status, err := strconv.ParseInt(values[0], 10, 16)
			if err != nil || !domain.PostStatus(status).Valid() {
				return httpx.BadRequest("status 必须为 all、0、1 或 2")
			}
			value := domain.PostStatus(status)
			listQuery.Status = &value
		}
	}
	return respondPosts(w, r, h.postUsecase.ListAdmin, listQuery)
}

type postStatusInput struct {
	Status *domain.PostStatus `json:"status" validate:"required"`
}

func (h *postHandler) setPostStatus(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	input, err := httpx.Body[postStatusInput](r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	err = h.postUsecase.SetStatus(
		r.Context(),
		actorFromPrincipal(principal),
		id, *input.Status,
	)
	if err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *postHandler) lockPostComments(w http.ResponseWriter, r *http.Request) error {
	return h.setPostCommentsLocked(w, r, true)
}

func (h *postHandler) unlockPostComments(w http.ResponseWriter, r *http.Request) error {
	return h.setPostCommentsLocked(w, r, false)
}

func (h *postHandler) setPostCommentsLocked(w http.ResponseWriter, r *http.Request, locked bool) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	err = h.postUsecase.SetCommentsLocked(
		r.Context(),
		actorFromPrincipal(principal),
		id, locked,
	)
	if err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type postPinInput struct {
	PinOrder *int32 `json:"pinOrder" validate:"required"`
}

func (h *postHandler) pinPost(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	input, err := httpx.Body[postPinInput](r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	err = h.postUsecase.SetPinOrder(
		r.Context(),
		actorFromPrincipal(principal),
		id, input.PinOrder,
	)
	if err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *postHandler) unpinPost(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	err = h.postUsecase.SetPinOrder(
		r.Context(),
		actorFromPrincipal(principal),
		id, nil,
	)
	if err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
