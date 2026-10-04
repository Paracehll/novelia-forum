package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	forumcategory "auth/internal/category"
	"auth/internal/domainfilter"
	"auth/internal/httpx"
	"auth/internal/repository"
	"auth/internal/usecase"

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
	postRepo       repository.PostRepository
	favoriteRepo   repository.FavoriteRepository
	commentUsecase *usecase.CommentUsecase
	domains        *domainfilter.Filter
}

func NewPostHandler(
	postRepo repository.PostRepository,
	favoriteRepo repository.FavoriteRepository,
	commentUsecase *usecase.CommentUsecase,
	domains *domainfilter.Filter,
) *postHandler {
	return &postHandler{
		postRepo:       postRepo,
		favoriteRepo:   favoriteRepo,
		commentUsecase: commentUsecase,
		domains:        domains,
	}
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

func respondPosts(
	w http.ResponseWriter,
	r *http.Request,
	repo repository.PostRepository,
	favoriteRepo repository.FavoriteRepository,
	filter repository.PostFilter,
) error {
	pagination, err := parsePagination(r.URL.Query(), 20, 100)
	if err != nil {
		return err
	}
	total, items, err := repo.List(filter, pagination.Limit, pagination.Offset)
	if err != nil {
		return repoError(err, "查询帖子失败")
	}
	favorites := map[int64]bool{}
	if principal, principalErr := httpx.AuthenticatedPrincipal(r); principalErr == nil {
		postIDs := make([]int64, len(items))
		for i, item := range items {
			postIDs[i] = item.ID
		}
		favorites, err = favoriteRepo.ListPostIDs(principal.UserID, postIDs)
		if err != nil {
			return repoError(err, "查询收藏状态失败")
		}
	}
	response := make([]postListItemResponse, len(items))
	for i, item := range items {
		response[i] = newPostListItemResponse(item, favorites[item.ID])
	}
	render.JSON(w, r, page[postListItemResponse]{Total: total, Items: response})
	return nil
}

func (h *postHandler) list(w http.ResponseWriter, r *http.Request) error {
	filter, err := postFilterFrom(r)
	if err != nil {
		return err
	}
	return respondPosts(w, r, h.postRepo, h.favoriteRepo, filter)
}

func (h *postHandler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	post, err := h.postRepo.Find(id, true)
	if err != nil {
		return repoError(err, "查询帖子失败")
	}
	favorited := false
	if principal, principalErr := httpx.AuthenticatedPrincipal(r); principalErr == nil {
		favorited, err = h.favoriteRepo.Has(post.ID, principal.UserID)
		if err != nil {
			return repoError(err, "查询收藏状态失败")
		}
	}
	render.JSON(w, r, newPostResponse(*post, favorited))
	return nil
}

type postInput struct {
	CategoryID int64   `json:"categoryId"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	TagIDs     []int64 `json:"tagIds"`
}

func (h *postHandler) validatePost(input postInput) error {
	if input.CategoryID <= 0 {
		return httpx.BadRequest("categoryId 必须为正整数")
	}
	if !validText(input.Title, 2, 100) {
		return httpx.BadRequest("title 长度必须为 2 到 100 字")
	}
	if !validText(input.Content, 1, 20000) || utf8.RuneCountInString(input.Content) > 20000 {
		return httpx.BadRequest("content 不能为空且不能超过 20000 字")
	}
	if !uniquePositiveIDs(input.TagIDs) {
		return httpx.BadRequest("tagIds 必须为不重复的正整数")
	}
	if len(input.TagIDs) > 3 {
		return httpx.BadRequest("一个帖子最多只能添加 3 个标签")
	}
	if err := checkDomainText(h.domains, "title", input.Title); err != nil {
		return err
	}
	return checkDomainText(h.domains, "content", input.Content)
}

func (h *postHandler) create(w http.ResponseWriter, r *http.Request) error {
	input, err := httpx.Body[postInput](r)
	if err != nil {
		return err
	}
	if err := h.validatePost(input); err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if input.CategoryID == forumcategory.AnnouncementsID && !principal.IsAdmin() {
		return httpx.Forbidden("站务公告仅管理员可以发帖")
	}
	post, err := h.postRepo.Create(repository.CreatePostInput{
		CategoryID:     input.CategoryID,
		Title:          strings.TrimSpace(input.Title),
		Content:        input.Content,
		TagIDs:         input.TagIDs,
		AuthorID:       principal.UserID,
		AuthorUsername: principal.Username,
		Attr:           "{}",
	})
	if err != nil {
		return repoError(err, "创建帖子失败")
	}
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, newPostResponse(*post, false))
	return nil
}

func (h *postHandler) ownedPost(r *http.Request) (*repository.PostDetails, error) {
	id, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return nil, err
	}
	post, err := h.postRepo.Find(id, false)
	if err != nil {
		return nil, repoError(err, "查询帖子失败")
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if post.AuthorID != principal.UserID && !principal.IsAdmin() {
		return nil, httpx.Forbidden("只能修改自己的帖子")
	}
	return post, nil
}

func (h *postHandler) update(w http.ResponseWriter, r *http.Request) error {
	ownedPost, err := h.ownedPost(r)
	if err != nil {
		return err
	}
	input, err := httpx.Body[postInput](r)
	if err != nil {
		return err
	}
	if err := h.validatePost(input); err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if input.CategoryID == forumcategory.AnnouncementsID && !principal.IsAdmin() {
		return httpx.Forbidden("站务公告仅管理员可以发帖")
	}
	post, err := h.postRepo.Update(ownedPost.ID, repository.UpdatePostInput{
		CategoryID: input.CategoryID,
		Title:      strings.TrimSpace(input.Title),
		Content:    input.Content,
		TagIDs:     input.TagIDs,
	})
	if err != nil {
		return repoError(err, "更新帖子失败")
	}
	favorited, favoriteErr := h.favoriteRepo.Has(post.ID, principal.UserID)
	if favoriteErr != nil {
		return repoError(favoriteErr, "查询收藏状态失败")
	}
	render.JSON(w, r, newPostResponse(*post, favorited))
	return nil
}

func (h *postHandler) delete(w http.ResponseWriter, r *http.Request) error {
	post, err := h.ownedPost(r)
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if !principal.IsAdmin() && time.Now().After(post.CreatedAt.Add(20*time.Minute)) {
		return httpx.Forbidden("帖子只能在发布后 20 分钟内删除")
	}
	if err := h.postRepo.SetStatus(post.ID, repository.StatusDeleted); err != nil {
		return repoError(err, "删除帖子失败")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *postHandler) setFavorite(w http.ResponseWriter, r *http.Request, favorite bool) error {
	postID, err := httpx.ParseParamPositiveInt(r, "id")
	if err != nil {
		return err
	}
	principal, _ := httpx.AuthenticatedPrincipal(r)
	if err := h.favoriteRepo.Set(postID, principal.UserID, favorite); err != nil {
		return repoError(err, "更新收藏失败")
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
	total, items, err := h.commentUsecase.ListRoots(
		commentActor(principal),
		repository.CommentSubjectPost,
		repository.PostSubjectKey(postID),
		pagination.Limit,
		pagination.Offset,
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
		repository.CommentSubjectPost,
		repository.PostSubjectKey(postID),
		rootID,
		pagination.Limit,
		pagination.Offset,
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
	comment, err := h.commentUsecase.CreatePost(repository.CreateCommentInput{
		SubjectKey:     repository.PostSubjectKey(postID),
		RootID:         input.RootID,
		Content:        input.Content,
		AuthorID:       principal.UserID,
		AuthorUsername: principal.Username,
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
