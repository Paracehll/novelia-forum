package usecase

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/domainfilter"
	"forum/internal/repository"
)

const (
	CodePostIDInvalid              = "post.id_invalid"
	CodePostTitleInvalid           = "post.title_invalid"
	CodePostContentInvalid         = "post.content_invalid"
	CodePostCategoryInvalid        = "post.category_invalid"
	CodePostTagInvalid             = "post.tag_invalid"
	CodePostSortInvalid            = "post.sort_invalid"
	CodePostPaginationInvalid      = "post.pagination_invalid"
	CodePostStatusInvalid          = "post.status_invalid"
	CodePostAuthorInvalid          = "post.author_invalid"
	CodePostNotFound               = "post.not_found"
	CodePostConflict               = "post.conflict"
	CodePostNotOwner               = "post.not_owner"
	CodePostAdminRequired          = "post.admin_required"
	CodePostAuthRequired           = "post.auth_required"
	CodePostAnnouncementRestricted = "post.announcement_restricted"
	CodePostDeleteExpired          = "post.delete_expired"
	CodePostDomainBlocked          = "post.domain_blocked"
	CodePostDomainInvalid          = "post.domain_invalid"
)

type PostUsecase struct {
	postRepo     repository.PostRepository
	favoriteRepo repository.FavoriteRepository
	domains      *domainfilter.Filter
}

func NewPostUsecase(
	postRepo repository.PostRepository,
	favoriteRepo repository.FavoriteRepository,
	domains *domainfilter.Filter,
) *PostUsecase {
	return &PostUsecase{
		postRepo:     postRepo,
		favoriteRepo: favoriteRepo,
		domains:      domains,
	}
}

type PostResult struct {
	Post      domain.Post
	Favorited bool
}

const (
	PostSortActive   = "active"
	PostSortNewest   = "newest"
	PostSortViews    = "views"
	PostSortComments = "comments"
)

type ListPostsQuery struct {
	CategorySlug  string
	Search        string
	Sort          string
	TagIDs        []int64
	AuthorName    string
	AuthorID      int64
	Status        *domain.PostStatus // nil includes all statuses in admin lists.
	Limit, Offset int64
}

// Public and personal lists must never inherit privileged identity filters.
func publicPostQuery(query ListPostsQuery) ListPostsQuery {
	status := domain.PostStatusPublished
	query.Status = &status
	query.AuthorName = ""
	query.AuthorID = 0
	return query
}

func (u *PostUsecase) List(actor Actor, query ListPostsQuery) (int64, []domain.PostListItem, error) {
	return u.list(actor, publicPostQuery(query), 0)
}

func (u *PostUsecase) ListAdmin(actor Actor, query ListPostsQuery) (int64, []domain.PostListItem, error) {
	if err := checkPostAdmin(actor); err != nil {
		return 0, nil, err
	}
	if query.Status != nil && !query.Status.Valid() {
		return 0, nil, Invalid(CodePostStatusInvalid, "status 必须为 all、0、1 或 2")
	}
	if query.AuthorID < 0 {
		return 0, nil, Invalid(CodePostAuthorInvalid, "用户 ID 必须为正整数")
	}
	query.AuthorName = strings.TrimSpace(query.AuthorName)
	return u.list(actor, query, 0)
}

func (u *PostUsecase) ListMine(actor Actor, query ListPostsQuery) (int64, []domain.PostListItem, error) {
	if actor.UserID <= 0 {
		return 0, nil, PermissionDenied(CodePostAuthRequired, "需要登录")
	}
	query = publicPostQuery(query)
	query.AuthorID = actor.UserID
	return u.list(actor, query, 0)
}

func (u *PostUsecase) ListFavorites(actor Actor, query ListPostsQuery) (int64, []domain.PostListItem, error) {
	if actor.UserID <= 0 {
		return 0, nil, PermissionDenied(CodePostAuthRequired, "需要登录")
	}
	query = publicPostQuery(query)
	return u.list(actor, query, actor.UserID)
}

func (u *PostUsecase) list(actor Actor, query ListPostsQuery, favoriteUserID int64) (int64, []domain.PostListItem, error) {
	if query.Limit <= 0 || query.Limit > 100 || query.Offset < 0 {
		return 0, nil, Invalid(CodePostPaginationInvalid, "limit 必须为 1 到 100，offset 不能为负数")
	}
	if query.Sort == "" {
		query.Sort = PostSortActive
	}
	switch query.Sort {
	case PostSortActive, PostSortNewest, PostSortViews, PostSortComments:
	default:
		return 0, nil, Invalid(CodePostSortInvalid, "sort 必须为 active、newest、views 或 comments")
	}
	if !uniquePostTagIDs(query.TagIDs) {
		return 0, nil, Invalid(CodePostTagInvalid, "tag 必须为不重复的正整数")
	}
	query.Search = strings.TrimSpace(query.Search)
	status := domain.PostStatus(repository.PostStatusAll)
	if query.Status != nil {
		status = *query.Status
	}
	filter := repository.PostFilter{
		CategorySlug: query.CategorySlug, Search: query.Search, Sort: query.Sort,
		TagIDs: query.TagIDs, AuthorName: query.AuthorName, AuthorID: query.AuthorID,
		Status: status, FavoriteUserID: favoriteUserID,
	}
	total, posts, err := u.postRepo.List(filter, query.Limit, query.Offset)
	if err != nil {
		return 0, nil, fmt.Errorf("post.list: %w", err)
	}
	var favorites map[int64]bool
	if actor.UserID > 0 {
		ids := make([]int64, len(posts))
		for i, post := range posts {
			ids[i] = post.ID
		}
		favorites, err = u.favoriteRepo.ListPostIDs(actor.UserID, ids)
		if err != nil {
			return 0, nil, fmt.Errorf("post.list_favorites: %w", err)
		}
	}
	for i := range posts {
		posts[i].Favorited = favorites[posts[i].ID]
	}
	return total, posts, nil
}

func (u *PostUsecase) Get(actor Actor, id int64) (*PostResult, error) {
	if err := checkPostID(id); err != nil {
		return nil, err
	}
	post, err := u.postRepo.Find(id, true)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("post.get: %w", err)
	}
	return u.result(actor, post)
}

func (u *PostUsecase) result(actor Actor, post *domain.Post) (*PostResult, error) {
	favorited := false
	if actor.UserID > 0 {
		var err error
		favorited, err = u.favoriteRepo.Has(post.ID, actor.UserID)
		if err != nil {
			return nil, fmt.Errorf("post.get_favorite: %w", err)
		}
	}
	return &PostResult{Post: *post, Favorited: favorited}, nil
}

type PostInput struct {
	CategoryID     int64
	Title, Content string
	TagIDs         []int64
}

func (u *PostUsecase) validateInput(actor Actor, input PostInput) error {
	if input.CategoryID <= 0 {
		return Invalid(CodePostCategoryInvalid, "categoryId 必须为正整数")
	}
	length := utf8.RuneCountInString(strings.TrimSpace(input.Title))
	if length < 2 || length > 100 {
		return Invalid(CodePostTitleInvalid, "title 长度必须为 2 到 100 字")
	}
	if strings.TrimSpace(input.Content) == "" || utf8.RuneCountInString(input.Content) > 20000 {
		return Invalid(CodePostContentInvalid, "content 不能为空且不能超过 20000 字")
	}
	if !uniquePostTagIDs(input.TagIDs) {
		return Invalid(CodePostTagInvalid, "tagIds 必须为不重复的正整数")
	}
	if len(input.TagIDs) > 3 {
		return Invalid(CodePostTagInvalid, "一个帖子最多只能添加 3 个标签")
	}
	if err := u.checkDomainText("title", input.Title); err != nil {
		return err
	}
	if err := u.checkDomainText("content", input.Content); err != nil {
		return err
	}
	if input.CategoryID == forumcategory.AnnouncementsID && !actor.IsAdmin {
		return PermissionDenied(CodePostAnnouncementRestricted, "站务公告仅管理员可以发帖")
	}
	return nil
}

func uniquePostTagIDs(ids []int64) bool {
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (u *PostUsecase) checkDomainText(field, text string) error {
	if u.domains == nil {
		return nil
	}
	switch err := u.domains.Check(text); {
	case err == nil:
		return nil
	case errors.Is(err, domainfilter.ErrBlocked):
		return Invalid(CodePostDomainBlocked, field+" 包含禁止使用的域名")
	case errors.Is(err, domainfilter.ErrText), errors.Is(err, domainfilter.ErrCandidate):
		return Invalid(CodePostDomainInvalid, field+" 无法完成域名检查")
	default:
		return fmt.Errorf("post.check_domain: %w", err)
	}
}

func (u *PostUsecase) Create(actor Actor, input PostInput) (*PostResult, error) {
	if err := u.validateInput(actor, input); err != nil {
		return nil, err
	}
	post, err := u.postRepo.Create(repository.CreatePostInput{
		CategoryID: input.CategoryID, Title: strings.TrimSpace(input.Title), Content: input.Content,
		TagIDs: input.TagIDs, AuthorID: actor.UserID, AuthorUsername: actor.Username, Attr: "{}",
	})
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrInvalidCategory):
		return nil, Invalid(CodePostCategoryInvalid, "分类无效")
	case errors.Is(err, repository.ErrInvalidTag):
		return nil, Invalid(CodePostTagInvalid, "标签无效")
	case errors.Is(err, repository.ErrConflict):
		return nil, Conflict(CodePostConflict, "帖子数据冲突")
	default:
		return nil, fmt.Errorf("post.create: %w", err)
	}
	return &PostResult{Post: *post}, nil
}

func (u *PostUsecase) ownedPost(actor Actor, id int64) (*domain.Post, error) {
	if err := checkPostID(id); err != nil {
		return nil, err
	}
	post, err := u.postRepo.Find(id, false)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("post.find: %w", err)
	}
	if !post.IsOwnedBy(actor.UserID) && !actor.IsAdmin {
		return nil, PermissionDenied(CodePostNotOwner, "只能修改自己的帖子")
	}
	return post, nil
}

func (u *PostUsecase) Update(actor Actor, id int64, input PostInput) (*PostResult, error) {
	if _, err := u.ownedPost(actor, id); err != nil {
		return nil, err
	}
	if err := u.validateInput(actor, input); err != nil {
		return nil, err
	}
	post, err := u.postRepo.Update(id, repository.UpdatePostInput{
		CategoryID: input.CategoryID, Title: strings.TrimSpace(input.Title), Content: input.Content, TagIDs: input.TagIDs,
	})
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		// The post existed above; it changed or disappeared before the write.
		return nil, Conflict(CodePostConflict, "帖子数据已变化，请刷新后重试")
	case errors.Is(err, repository.ErrInvalidCategory):
		return nil, Invalid(CodePostCategoryInvalid, "分类无效")
	case errors.Is(err, repository.ErrInvalidTag):
		return nil, Invalid(CodePostTagInvalid, "标签无效")
	case errors.Is(err, repository.ErrConflict):
		return nil, Conflict(CodePostConflict, "帖子数据冲突")
	default:
		return nil, fmt.Errorf("post.update: %w", err)
	}
	return u.result(actor, post)
}

func (u *PostUsecase) Delete(actor Actor, id int64) error {
	post, err := u.ownedPost(actor, id)
	if err != nil {
		return err
	}
	if !actor.IsAdmin && !post.WithinDeletionWindow(time.Now()) {
		return PermissionDenied(CodePostDeleteExpired, "帖子只能在发布后 20 分钟内删除")
	}
	err = u.postRepo.SetStatus(id, domain.PostStatusDeleted)
	if errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return fmt.Errorf("post.delete: %w", err)
	}
	return nil
}

func (u *PostUsecase) SetFavorite(actor Actor, id int64, favorite bool) error {
	if err := checkPostID(id); err != nil {
		return err
	}
	if actor.UserID <= 0 {
		return PermissionDenied(CodePostAuthRequired, "需要登录")
	}
	err := u.favoriteRepo.Set(id, actor.UserID, favorite)
	if favorite && errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return fmt.Errorf("post.set_favorite: %w", err)
	}
	return nil
}

func checkPostAdmin(actor Actor) error {
	if !actor.IsAdmin {
		return PermissionDenied(CodePostAdminRequired, "需要管理员权限")
	}
	return nil
}

func checkPostID(id int64) error {
	if id <= 0 {
		return Invalid(CodePostIDInvalid, "id 必须为正整数")
	}
	return nil
}

func (u *PostUsecase) SetStatus(actor Actor, id int64, status domain.PostStatus) error {
	if err := checkPostAdmin(actor); err != nil {
		return err
	}
	if err := checkPostID(id); err != nil {
		return err
	}
	if !status.Valid() {
		return Invalid(CodePostStatusInvalid, "status 必须为 0、1 或 2")
	}
	err := u.postRepo.SetStatus(id, status)
	if errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return fmt.Errorf("post.set_status: %w", err)
	}
	return nil
}

func (u *PostUsecase) SetCommentsLocked(actor Actor, id int64, locked bool) error {
	if err := checkPostAdmin(actor); err != nil {
		return err
	}
	if err := checkPostID(id); err != nil {
		return err
	}
	err := u.postRepo.SetCommentsLocked(id, locked)
	if errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return fmt.Errorf("post.set_comments_locked: %w", err)
	}
	return nil
}

func (u *PostUsecase) SetPinOrder(actor Actor, id int64, pinOrder *int32) error {
	if err := checkPostAdmin(actor); err != nil {
		return err
	}
	if err := checkPostID(id); err != nil {
		return err
	}
	// Any int32 order is supported; nil removes the pin, as in the handler.
	err := u.postRepo.SetPinOrder(id, pinOrder)
	if errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodePostNotFound, "帖子不存在")
	}
	if err != nil {
		return fmt.Errorf("post.set_pin_order: %w", err)
	}
	return nil
}
