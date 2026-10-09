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

type externalCommentHandler struct {
	commentUsecase *usecase.CommentUsecase
}

func NewExternalCommentHandler(
	commentUsecase *usecase.CommentUsecase,
) *externalCommentHandler {
	return &externalCommentHandler{
		commentUsecase: commentUsecase,
	}
}

func (h *externalCommentHandler) RegisterRoutes(router chi.Router) {
	router.Get("/{type}/{subjectKey}", httpx.EH(h.list))
	router.Get("/{type}/{subjectKey}/{rootId}/reply", httpx.EH(h.listReplies))
	router.With(httpx.RequireMember).Post("/{type}/{subjectKey}", httpx.EH(h.create))
	router.With(httpx.RequireMember).Patch("/{type}/{commentId}", httpx.EH(h.update))
	router.With(httpx.RequireAccessToken).Delete("/{type}/{commentId}", httpx.EH(h.delete))
	router.With(httpx.RequireAdmin).Put("/{type}/{commentId}/status", httpx.EH(h.setStatus))
}

type externalCommentResponse struct {
	ID             int64                          `json:"id"`
	SubjectKey     string                         `json:"subjectKey"`
	RootID         *int64                         `json:"rootId"`
	Content        string                         `json:"content"`
	AuthorID       int64                          `json:"authorId"`
	AuthorUsername string                         `json:"authorUsername"`
	AuthorBlocked  bool                           `json:"authorBlocked"`
	Status         int16                          `json:"status"`
	CreatedAt      time.Time                      `json:"createdAt"`
	UpdatedAt      time.Time                      `json:"updatedAt"`
	ReplyCount     int64                          `json:"replyCount"`
	Replies        *page[externalCommentResponse] `json:"replies,omitempty"`
}

func newExternalCommentThreadResponse(value domain.CommentThreadPreview) externalCommentResponse {
	response := newExternalCommentResponse(value.Root)
	response.ReplyCount = value.ReplyCount
	items := make([]externalCommentResponse, len(value.Replies))
	for i, reply := range value.Replies {
		items[i] = newExternalCommentResponse(reply)
	}
	response.Replies = &page[externalCommentResponse]{Total: value.ReplyCount, Items: items}
	return response
}

func newExternalCommentResponse(value domain.CommentReadModel) externalCommentResponse {
	return externalCommentResponse{
		ID:             value.ID,
		SubjectKey:     value.SubjectKey,
		RootID:         value.RootID,
		Content:        value.Content,
		AuthorID:       value.AuthorID,
		AuthorUsername: value.AuthorUsername,
		AuthorBlocked:  value.AuthorBlocked,
		Status:         int16(value.Status),
		CreatedAt:      value.CreatedAt,
		UpdatedAt:      value.UpdatedAt,
	}
}

func externalCommentSubjectKey(r *http.Request) (string, error) {
	subjectKey := chi.URLParam(r, "subjectKey")
	if !validText(subjectKey, 1, 255) {
		return "", httpx.BadRequest("subjectKey 长度必须为 1 到 255")
	}
	return subjectKey, nil
}

func (h *externalCommentHandler) list(w http.ResponseWriter, r *http.Request) error {
	subjectKey, err := externalCommentSubjectKey(r)
	if err != nil {
		return err
	}
	pagination, err := parsePagination(r.URL.Query(), 20, 100)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	total, items, err := h.commentUsecase.ListExternal(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.ListExternalCommentsQuery{
			Kind:       chi.URLParam(r, "type"),
			SubjectKey: subjectKey,
			Limit:      pagination.Limit,
			Offset:     pagination.Offset,
		},
	)
	if err != nil {
		return transportError(err)
	}
	response := make([]externalCommentResponse, len(items))
	for i, item := range items {
		response[i] = newExternalCommentThreadResponse(item)
	}
	render.JSON(w, r, page[externalCommentResponse]{Total: total, Items: response})
	return nil
}

func (h *externalCommentHandler) listReplies(w http.ResponseWriter, r *http.Request) error {
	subjectKey, err := externalCommentSubjectKey(r)
	if err != nil {
		return err
	}
	rootID, err := httpx.ParseParamPositiveInt(r, "rootId")
	if err != nil {
		return err
	}
	pagination, err := parsePagination(r.URL.Query(), 20, 100)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	total, items, err := h.commentUsecase.ListExternalReplies(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.ListExternalCommentRepliesQuery{
			Kind:       chi.URLParam(r, "type"),
			SubjectKey: subjectKey,
			RootID:     rootID,
			Limit:      pagination.Limit,
			Offset:     pagination.Offset,
		},
	)
	if err != nil {
		return transportError(err)
	}
	response := make([]externalCommentResponse, len(items))
	for i, item := range items {
		response[i] = newExternalCommentResponse(item)
	}
	render.JSON(w, r, page[externalCommentResponse]{Total: total, Items: response})
	return nil
}

func (h *externalCommentHandler) create(w http.ResponseWriter, r *http.Request) error {
	subjectKey, err := externalCommentSubjectKey(r)
	if err != nil {
		return err
	}
	input, err := httpx.Body[commentInput](r)
	if err != nil {
		return err
	}
	if err := validateComment(input); err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	comment, err := h.commentUsecase.CreateExternal(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.CreateExternalCommentCommand{
			Kind:       chi.URLParam(r, "type"),
			SubjectKey: subjectKey,
			RootID:     input.RootID,
			Content:    input.Content,
		},
	)
	if err != nil {
		return transportError(err)
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, newExternalCommentResponse(*comment))
	return nil
}

func (h *externalCommentHandler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "commentId")
	if err != nil {
		return err
	}
	input, err := httpx.Body[commentUpdateInput](r)
	if err != nil {
		return err
	}
	if err := validateCommentUpdate(input); err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	comment, err := h.commentUsecase.UpdateExternal(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.UpdateExternalCommentCommand{
			Kind:      chi.URLParam(r, "type"),
			CommentID: id,
			Content:   input.Content,
		},
	)
	if err != nil {
		return transportError(err)
	}
	render.JSON(w, r, newExternalCommentResponse(*comment))
	return nil
}

func (h *externalCommentHandler) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "commentId")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.commentUsecase.DeleteExternal(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.DeleteExternalCommentCommand{
			Kind:      chi.URLParam(r, "type"),
			CommentID: id,
		},
	); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *externalCommentHandler) setStatus(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "commentId")
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
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.commentUsecase.SetExternalStatus(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.SetExternalCommentStatusCommand{
			Kind:      chi.URLParam(r, "type"),
			CommentID: id,
			Status:    status,
		},
	); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
