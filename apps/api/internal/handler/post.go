package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"forum/internal/domain"
	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type postTagResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color int16  `json:"color"`
}

type postListItemResponse struct {
	ID             int64             `json:"id"`
	CategoryID     int64             `json:"categoryId"`
	Title          string            `json:"title"`
	AuthorID       int64             `json:"authorId"`
	AuthorUsername string            `json:"authorUsername"`
	Status         int16             `json:"status"`
	ViewsCount     int32             `json:"viewsCount"`
	CommentsCount  int32             `json:"commentsCount"`
	CommentsLocked bool              `json:"commentsLocked"`
	PinOrder       *int32            `json:"pinOrder"`
	Favorited      bool              `json:"favorited"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	ActiveAt       time.Time         `json:"activeAt"`
	Tags           []postTagResponse `json:"tags"`
}

type postResponse struct {
	postListItemResponse
	Content string `json:"content"`
}

func newPostResponse(value repository.PostDetails, favorited bool) postResponse {
	return postResponse{
		postListItemResponse: newPostListItemResponse(value, favorited),
		Content:              value.Content,
	}
}

func newPostListItemResponse(value repository.PostDetails, favorited bool) postListItemResponse {
	tags := make([]postTagResponse, len(value.Tags))
	for i, tag := range value.Tags {
		tags[i] = postTagResponse{
			ID:    tag.ID,
			Name:  tag.Name,
			Color: tag.Color,
		}
	}
	return postListItemResponse{
		ID:             value.ID,
		CategoryID:     value.CategoryID,
		Title:          value.Title,
		AuthorID:       value.AuthorID,
		AuthorUsername: value.AuthorUsername,
		Status:         value.Status,
		ViewsCount:     value.ViewsCount,
		CommentsCount:  value.CommentsCount,
		CommentsLocked: value.CommentsLocked,
		PinOrder:       value.PinOrder,
		Favorited:      favorited,
		CreatedAt:      value.CreatedAt,
		UpdatedAt:      value.UpdatedAt,
		ActiveAt:       value.ActiveAt,
		Tags:           tags,
	}
}

type postHandler struct {
	postUsecase    *usecase.PostUsecase
	commentUsecase *usecase.CommentUsecase
}

func NewPostHandler(posts *usecase.PostUsecase, comments *usecase.CommentUsecase) *postHandler {
	return &postHandler{postUsecase: posts, commentUsecase: comments}
}

func (h *postHandler) RegisterRoutes(router chi.Router) {
	router.Get("/", httpx.EH(h.list))
	router.With(httpx.RequireMember).Post("/", httpx.EH(h.create))
	router.Route("/{id}", func(router chi.Router) {
		router.Get("/", httpx.EH(h.get))
		router.With(httpx.RequireMember).Patch("/", httpx.EH(h.update))
		router.With(httpx.RequireAccessToken).Delete("/", httpx.EH(h.delete))
		router.With(httpx.RequireAccessToken).Put("/favorite", httpx.EH(h.favorite))
		router.With(httpx.RequireAccessToken).Delete("/favorite", httpx.EH(h.unfavorite))
		router.Get("/comment", httpx.EH(h.listComments))
		router.With(httpx.RequireMember).Post("/comment", httpx.EH(h.createComment))
		router.Get("/comment/{rootId}/reply", httpx.EH(h.listCommentReplies))
	})
}

func postFilterFrom(r *http.Request) (repository.PostFilter, error) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = repository.PostSortActive
	}
	switch sort {
	case repository.PostSortActive, repository.PostSortNewest, repository.PostSortViews, repository.PostSortComments:
	default:
		return repository.PostFilter{}, httpx.BadRequest("sort 必须为 active、newest、views 或 comments")
	}
	filter := repository.PostFilter{
		CategorySlug: r.URL.Query().Get("category"),
		Search:       strings.TrimSpace(r.URL.Query().Get("q")),
		Sort:         sort,
	}
	for _, part := range r.URL.Query()["tag"] {
		for _, value := range strings.Split(part, ",") {
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id <= 0 {
				return filter, httpx.BadRequest("tag 必须为正整数")
			}
			filter.TagIDs = append(filter.TagIDs, id)
		}
	}
	if !uniquePositiveIDs(filter.TagIDs) {
		return filter, httpx.BadRequest("tag 不能重复")
	}
	return filter, nil
}

type postListFunc func(usecase.Actor, usecase.ListPostsQuery) (int64, []usecase.PostResult, error)

func respondPosts(w http.ResponseWriter, r *http.Request, list postListFunc, filter repository.PostFilter) error {
	pagination, err := parsePagination(r.URL.Query(), 20, 100)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	total, items, err := list(commentActor(principal), usecase.ListPostsQuery{
		Filter: filter, Limit: pagination.Limit, Offset: pagination.Offset,
	})
	if err != nil {
		return transportError(err)
	}
	response := make([]postListItemResponse, len(items))
	for i, item := range items {
		response[i] = newPostListItemResponse(item.Post, item.Favorited)
	}
	render.JSON(w, r, page[postListItemResponse]{Total: total, Items: response})
	return nil
}

func (h *postHandler) list(w http.ResponseWriter, r *http.Request) error {
	filter, err := postFilterFrom(r)
	if err != nil {
		return err
	}
	return respondPosts(w, r, h.postUsecase.List, filter)
}

func (h *postHandler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	post, err := h.postUsecase.Get(commentActor(principal), id)
	if err != nil {
		return transportError(err)
	}
	render.JSON(w, r, newPostResponse(post.Post, post.Favorited))
	return nil
}

type postInput struct {
	CategoryID int64   `json:"categoryId"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	TagIDs     []int64 `json:"tagIds"`
}

func (input postInput) command() usecase.PostInput {
	return usecase.PostInput{CategoryID: input.CategoryID, Title: input.Title, Content: input.Content, TagIDs: input.TagIDs}
}

func (h *postHandler) create(w http.ResponseWriter, r *http.Request) error {
	input, err := httpx.Body[postInput](r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	post, err := h.postUsecase.Create(commentActor(principal), input.command())
	if err != nil {
		return transportError(err)
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, newPostResponse(post.Post, post.Favorited))
	return nil
}

func (h *postHandler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	input, err := httpx.Body[postInput](r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	post, err := h.postUsecase.Update(commentActor(principal), id, input.command())
	if err != nil {
		return transportError(err)
	}
	render.JSON(w, r, newPostResponse(post.Post, post.Favorited))
	return nil
}

func (h *postHandler) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.postUsecase.Delete(commentActor(principal), id); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *postHandler) setFavorite(w http.ResponseWriter, r *http.Request, favorite bool) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.postUsecase.SetFavorite(commentActor(principal), id, favorite); err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *postHandler) favorite(w http.ResponseWriter, r *http.Request) error {
	return h.setFavorite(w, r, true)
}

func (h *postHandler) unfavorite(w http.ResponseWriter, r *http.Request) error {
	return h.setFavorite(w, r, false)
}

func (h *postHandler) listComments(w http.ResponseWriter, r *http.Request) error {
	postID, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	pagination, err := parsePagination(r.URL.Query(), 20, 100)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	total, items, err := h.commentUsecase.List(
		commentActor(principal),
		usecase.ListCommentsQuery{
			SubjectType: domain.CommentSubjectPost,
			SubjectKey:  domain.PostCommentSubjectKey(postID),
			Limit:       pagination.Limit,
			Offset:      pagination.Offset,
		},
	)
	if err != nil {
		return transportError(err)
	}
	response := make([]commentResponse, len(items))
	for i, item := range items {
		response[i], err = newCommentThreadResponse(item)
		if err != nil {
			return httpx.InternalError(err, "转换评论数据失败")
		}
	}
	render.JSON(w, r, page[commentResponse]{Total: total, Items: response})
	return nil
}

func (h *postHandler) listCommentReplies(w http.ResponseWriter, r *http.Request) error {
	postID, err := httpx.ParseParamPositiveInt(r, "id")
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
	total, items, err := h.commentUsecase.ListReplies(
		commentActor(principal),
		usecase.ListCommentRepliesQuery{
			SubjectType: domain.CommentSubjectPost,
			SubjectKey:  domain.PostCommentSubjectKey(postID),
			RootID:      rootID,
			Limit:       pagination.Limit,
			Offset:      pagination.Offset,
		},
	)
	if err != nil {
		return transportError(err)
	}
	response := make([]commentResponse, len(items))
	for i, item := range items {
		response[i], err = newCommentResponse(item)
		if err != nil {
			return httpx.InternalError(err, "转换评论回复数据失败")
		}
	}
	render.JSON(w, r, page[commentResponse]{Total: total, Items: response})
	return nil
}

func (h *postHandler) createComment(w http.ResponseWriter, r *http.Request) error {
	postID, err := httpx.ParseParamPositiveInt(r, "id")
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
	comment, err := h.commentUsecase.Create(commentActor(principal), usecase.CreatePostCommentCommand{
		PostID:  postID,
		RootID:  input.RootID,
		Content: input.Content,
	})
	if err != nil {
		return transportError(err)
	}
	response, err := newCommentResponse(*comment)
	if err != nil {
		return httpx.InternalError(err, "转换评论数据失败")
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, response)
	return nil
}
