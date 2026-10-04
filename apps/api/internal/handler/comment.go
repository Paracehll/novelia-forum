package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type commentHandler struct{ commentUsecase *usecase.CommentUsecase }

func NewCommentHandler(commentUsecase *usecase.CommentUsecase) *commentHandler {
	return &commentHandler{commentUsecase: commentUsecase}
}

func (h *commentHandler) RegisterRoutes(router chi.Router) {
	router.With(httpx.RequireMember).Patch("/{id}", httpx.EH(h.update))
	router.With(httpx.RequireAccessToken).Delete("/{id}", httpx.EH(h.delete))
}

type commentInput struct {
	Content string `json:"content"`
	RootID  *int64 `json:"rootId"`
}

type commentUpdateInput struct {
	Content string          `json:"content"`
	RootID  json.RawMessage `json:"rootId"`
}

func validateCommentUpdate(input commentUpdateInput) error {
	if len(input.RootID) > 0 {
		return httpx.BadRequest("rootId 不能修改")
	}
	return nil
}

func validateComment(input commentInput) error {
	if input.RootID != nil && *input.RootID <= 0 {
		return httpx.BadRequest("rootId 必须为正整数")
	}
	return nil
}

type commentResponse struct {
	ID             int64                  `json:"id"`
	PostID         int64                  `json:"postId"`
	RootID         *int64                 `json:"rootId"`
	Content        string                 `json:"content"`
	AuthorID       int64                  `json:"authorId"`
	AuthorUsername string                 `json:"authorUsername"`
	Status         int16                  `json:"status"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
	ReplyCount     int64                  `json:"replyCount"`
	Replies        *page[commentResponse] `json:"replies,omitempty"`
}

func newCommentResponse(value domain.Comment) (commentResponse, error) {
	postID, err := domain.PostIDFromCommentSubjectKey(value.SubjectKey)
	if err != nil {
		return commentResponse{}, fmt.Errorf("comment %d: %w", value.ID, err)
	}
	return commentResponse{
		ID:             value.ID,
		PostID:         postID,
		RootID:         value.RootID,
		Content:        value.Content,
		AuthorID:       value.AuthorID,
		AuthorUsername: value.AuthorUsername,
		Status:         int16(value.Status),
		CreatedAt:      value.CreatedAt,
		UpdatedAt:      value.UpdatedAt,
	}, nil
}

func newCommentThreadResponse(value domain.CommentThreadPreview) (commentResponse, error) {
	response, err := newCommentResponse(value.Root)
	if err != nil {
		return response, err
	}
	response.ReplyCount = value.ReplyCount
	items := make([]commentResponse, len(value.Replies))
	for i, reply := range value.Replies {
		items[i], err = newCommentResponse(reply)
		if err != nil {
			return response, err
		}
	}
	response.Replies = &page[commentResponse]{Total: value.ReplyCount, Items: items}
	return response, nil
}

func (h *commentHandler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
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
	comment, err := h.commentUsecase.Update(
		actorFromPrincipal(principal),
		usecase.UpdateCommentCommand{
			SubjectType: domain.CommentSubjectPost,
			CommentID:   id,
			Content:     input.Content,
		},
	)
	if err != nil {
		return transportError(err)
	}
	response, err := newCommentResponse(*comment)
	if err != nil {
		return httpx.InternalError(err, "转换评论数据失败")
	}
	render.JSON(w, r, response)
	return nil
}

func (h *commentHandler) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.commentUsecase.Delete(actorFromPrincipal(principal), usecase.DeleteCommentCommand{
		SubjectType: domain.CommentSubjectPost,
		CommentID:   id,
	}); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
