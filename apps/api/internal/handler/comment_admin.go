package handler

import (
	"net/http"
	"strconv"
	"strings"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

func (h *commentHandler) RegisterAdminRoutes(router chi.Router) {
	router.Get("/", httpx.EH(h.listAdmin))
	router.Put("/{id}/status", httpx.EH(h.setStatus))
	router.Delete("/author/{authorId}", httpx.EH(h.deleteAllByAuthor))
}

type commentStatusInput struct {
	Status string `json:"status" validate:"required"`
}

func (h *commentHandler) setStatus(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	input, err := httpx.Body[commentStatusInput](r)
	if err != nil {
		return err
	}
	status, err := parseCommentStatus(input.Status)
	if err != nil {
		return err
	}
	if err := h.commentUsecase.SetStatus(usecase.SetCommentStatusCommand{
		SubjectType: domain.CommentSubjectPost,
		CommentID:   id,
		Status:      status,
	}); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *commentHandler) deleteAllByAuthor(w http.ResponseWriter, r *http.Request) error {
	authorID, err := httpx.ParseParamPositiveInt(r, "authorId")
	if err != nil {
		return err
	}
	if err := h.commentUsecase.DeleteAllByAuthor(usecase.DeleteCommentsByAuthorCommand{AuthorID: authorID}); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *commentHandler) listAdmin(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	pagination, err := parsePagination(query, 20, 100)
	if err != nil {
		return err
	}
	filter := usecase.ListAdminCommentsQuery{
		Search:     strings.TrimSpace(query.Get("q")),
		AuthorName: strings.TrimSpace(query.Get("author_name")),
		Limit:      pagination.Limit,
		Offset:     pagination.Offset,
	}
	if values, ok := query["post_id"]; ok {
		if len(values) != 1 {
			return httpx.BadRequest("post_id 必须为正整数")
		}
		id, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || id <= 0 {
			return httpx.BadRequest("post_id 必须为正整数")
		}
		filter.PostID = id
	}
	if values, ok := query["status"]; ok {
		if len(values) != 1 {
			return httpx.BadRequest("status 必须为 all、0、1 或 2")
		}
		if values[0] != "all" {
			status, err := strconv.ParseInt(values[0], 10, 16)
			if err != nil || !validStatus(int16(status)) {
				return httpx.BadRequest("status 必须为 all、0、1 或 2")
			}
			value := domain.CommentStatus(status)
			filter.Status = &value
		}
	}
	total, items, err := h.commentUsecase.ListAdmin(filter)
	if err != nil {
		return transportError(err)
	}
	responses := make([]commentResponse, len(items))
	for i, item := range items {
		response, err := newCommentResponse(item)
		if err != nil {
			return httpx.InternalError(err, "转换评论数据失败")
		}
		responses[i] = response
	}
	render.JSON(w, r, page[commentResponse]{Total: total, Items: responses})
	return nil
}

func parseCommentStatus(value string) (domain.CommentStatus, error) {
	switch value {
	case "published":
		return domain.CommentStatusPublished, nil
	case "hidden":
		return domain.CommentStatusHidden, nil
	case "deleted":
		return domain.CommentStatusDeleted, nil
	default:
		return 0, httpx.BadRequest("status 必须为 published、hidden 或 deleted")
	}
}
