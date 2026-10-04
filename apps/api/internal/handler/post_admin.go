package handler

import (
	"net/http"
	"strconv"
	"strings"

	"forum/internal/httpx"
	"forum/internal/repository"

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
	filter, err := postFilterFrom(r)
	if err != nil {
		return err
	}
	filter.Status = repository.PostStatusAll
	filter.AuthorName = strings.TrimSpace(r.URL.Query().Get("author_name"))
	if values, ok := r.URL.Query()["author_id"]; ok {
		if len(values) != 1 {
			return httpx.BadRequest("author_id 必须为正整数")
		}
		authorID, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || authorID <= 0 {
			return httpx.BadRequest("author_id 必须为正整数")
		}
		filter.AuthorID = authorID
	}
	if values, ok := r.URL.Query()["status"]; ok {
		if len(values) != 1 {
			return httpx.BadRequest("status 必须为 all、0、1 或 2")
		}
		if values[0] != "all" {
			status, err := strconv.ParseInt(values[0], 10, 16)
			if err != nil || !validStatus(int16(status)) {
				return httpx.BadRequest("status 必须为 all、0、1 或 2")
			}
			filter.Status = int16(status)
		}
	}
	return respondPosts(w, r, h.postRepo, h.favoriteRepo, filter)
}

type postStatusInput struct {
	Status int16 `json:"status"`
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
	if !validStatus(input.Status) {
		return httpx.BadRequest("status 必须为 0、1 或 2")
	}
	if err := h.postRepo.SetStatus(id, input.Status); err != nil {
		return repoError(err, "设置帖子状态失败")
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
	if err := h.postRepo.SetCommentsLocked(id, locked); err != nil {
		return repoError(err, "设置帖子评论锁定状态失败")
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
	if err := h.postRepo.SetPinOrder(id, input.PinOrder); err != nil {
		return repoError(err, "置顶帖子失败")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *postHandler) unpinPost(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	if err := h.postRepo.SetPinOrder(id, nil); err != nil {
		return repoError(err, "取消置顶帖子失败")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
