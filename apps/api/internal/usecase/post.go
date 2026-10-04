package usecase

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	forumcategory "forum/internal/category"
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

func NewPostUsecase(postRepo repository.PostRepository, favoriteRepo repository.FavoriteRepository, domains *domainfilter.Filter) *PostUsecase {
	return &PostUsecase{postRepo: postRepo, favoriteRepo: favoriteRepo, domains: domains}
}

type PostResult struct {
	Post      repository.PostDetails
	Favorited bool
}

type ListPostsQuery struct {
	Filter        repository.PostFilter
	Limit, Offset int64
}

// Public and personal lists must never inherit privileged identity filters.
func publicPostQuery(query ListPostsQuery) ListPostsQuery {
	query.Filter.Status = repository.StatusPublished
	query.Filter.AuthorName = ""
	query.Filter.AuthorID = 0
	query.Filter.FavoriteUserID = 0
	return query
}

func (u *PostUsecase) List(actor Actor, query ListPostsQuery) (int64, []PostResult, error) {
	return u.list(actor, publicPostQuery(query))
}

func (u *PostUsecase) ListAdmin(actor Actor, query ListPostsQuery) (int64, []PostResult, error) {
	if err := checkPostAdmin(actor); err != nil {
		return 0, nil, err
	}
	if query.Filter.Status != repository.PostStatusAll && !validPostStatus(query.Filter.Status) {
		return 0, nil, Invalid(CodePostStatusInvalid, "status 必须为 all、0、1 或 2")
	}
	if query.Filter.AuthorID < 0 || query.Filter.FavoriteUserID < 0 {
		return 0, nil, Invalid(CodePostAuthorInvalid, "用户 ID 必须为正整数")
	}
	query.Filter.AuthorName = strings.TrimSpace(query.Filter.AuthorName)
	return u.list(actor, query)
}

func (u *PostUsecase) ListMine(actor Actor, query ListPostsQuery) (int64, []PostResult, error) {
	if actor.UserID <= 0 {
		return 0, nil, PermissionDenied(CodePostAuthRequired, "需要登录")
	}
	query = publicPostQuery(query)
	query.Filter.AuthorID = actor.UserID
	return u.list(actor, query)
}

func (u *PostUsecase) ListFavorites(actor Actor, query ListPostsQuery) (int64, []PostResult, error) {
	if actor.UserID <= 0 {
		return 0, nil, PermissionDenied(CodePostAuthRequired, "需要登录")
	}
	query = publicPostQuery(query)
	query.Filter.FavoriteUserID = actor.UserID
	return u.list(actor, query)
}

func (u *PostUsecase) list(actor Actor, query ListPostsQuery) (int64, []PostResult, error) {
	if query.Limit <= 0 || query.Limit > 100 || query.Offset < 0 {
		return 0, nil, Invalid(CodePostPaginationInvalid, "limit 必须为 1 到 100，offset 不能为负数")
	}
	if query.Filter.Sort == "" {
		query.Filter.Sort = repository.PostSortActive
	}
	switch query.Filter.Sort {
	case repository.PostSortActive, repository.PostSortNewest, repository.PostSortViews, repository.PostSortComments:
	default:
		return 0, nil, Invalid(CodePostSortInvalid, "sort 必须为 active、newest、views 或 comments")
	}
	if !uniquePostTagIDs(query.Filter.TagIDs) {
		return 0, nil, Invalid(CodePostTagInvalid, "tag 必须为不重复的正整数")
	}
	query.Filter.Search = strings.TrimSpace(query.Filter.Search)
	total, posts, err := u.postRepo.List(query.Filter, query.Limit, query.Offset)
	if err != nil {
		return 0, nil, postError(err, "post.list")
	}
	var favorites map[int64]bool
	if actor.UserID > 0 {
		ids := make([]int64, len(posts))
		for i, post := range posts {
			ids[i] = post.ID
		}
		favorites, err = u.favoriteRepo.ListPostIDs(actor.UserID, ids)
		if err != nil {
			return 0, nil, postError(err, "post.list_favorites")
		}
	}
	results := make([]PostResult, len(posts))
	for i, post := range posts {
		results[i] = PostResult{Post: post, Favorited: favorites[post.ID]}
	}
	return total, results, nil
}

func (u *PostUsecase) Get(actor Actor, id int64) (*PostResult, error) {
	if err := checkPostID(id); err != nil {
		return nil, err
	}
	post, err := u.postRepo.Find(id, true)
	if err != nil {
		return nil, postError(err, "post.get")
	}
	return u.result(actor, post)
}

func (u *PostUsecase) result(actor Actor, post *repository.PostDetails) (*PostResult, error) {
	favorited := false
	if actor.UserID > 0 {
		var err error
		favorited, err = u.favoriteRepo.Has(post.ID, actor.UserID)
		if err != nil {
			return nil, postError(err, "post.get_favorite")
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
	if err != nil {
		return nil, postError(err, "post.create")
	}
	return &PostResult{Post: *post}, nil
}

func (u *PostUsecase) ownedPost(actor Actor, id int64) (*repository.PostDetails, error) {
	if err := checkPostID(id); err != nil {
		return nil, err
	}
	post, err := u.postRepo.Find(id, false)
	if err != nil {
		return nil, postError(err, "post.find")
	}
	if post.AuthorID != actor.UserID && !actor.IsAdmin {
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
	if err != nil {
		return nil, postError(err, "post.update")
	}
	return u.result(actor, post)
}

func (u *PostUsecase) Delete(actor Actor, id int64) error {
	post, err := u.ownedPost(actor, id)
	if err != nil {
		return err
	}
	if !actor.IsAdmin && time.Now().After(post.CreatedAt.Add(20*time.Minute)) {
		return PermissionDenied(CodePostDeleteExpired, "帖子只能在发布后 20 分钟内删除")
	}
	return postError(u.postRepo.SetStatus(id, repository.StatusDeleted), "post.delete")
}

func (u *PostUsecase) SetFavorite(actor Actor, id int64, favorite bool) error {
	if err := checkPostID(id); err != nil {
		return err
	}
	if actor.UserID <= 0 {
		return PermissionDenied(CodePostAuthRequired, "需要登录")
	}
	return postError(u.favoriteRepo.Set(id, actor.UserID, favorite), "post.set_favorite")
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

func validPostStatus(status int16) bool {
	return status >= repository.StatusPublished && status <= repository.StatusDeleted
}

func (u *PostUsecase) SetStatus(actor Actor, id int64, status int16) error {
	if err := checkPostAdmin(actor); err != nil {
		return err
	}
	if err := checkPostID(id); err != nil {
		return err
	}
	if !validPostStatus(status) {
		return Invalid(CodePostStatusInvalid, "status 必须为 0、1 或 2")
	}
	return postError(u.postRepo.SetStatus(id, status), "post.set_status")
}

func (u *PostUsecase) SetCommentsLocked(actor Actor, id int64, locked bool) error {
	if err := checkPostAdmin(actor); err != nil {
		return err
	}
	if err := checkPostID(id); err != nil {
		return err
	}
	return postError(u.postRepo.SetCommentsLocked(id, locked), "post.set_comments_locked")
}

func (u *PostUsecase) SetPinOrder(actor Actor, id int64, pinOrder *int32) error {
	if err := checkPostAdmin(actor); err != nil {
		return err
	}
	if err := checkPostID(id); err != nil {
		return err
	}
	// Any int32 order is supported; nil removes the pin, as in the handler.
	return postError(u.postRepo.SetPinOrder(id, pinOrder), "post.set_pin_order")
}

func postError(err error, operation string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound):
		return NotFound(CodePostNotFound, "帖子不存在")
	case errors.Is(err, repository.ErrInvalidCategory):
		return Invalid(CodePostCategoryInvalid, "分类无效")
	case errors.Is(err, repository.ErrInvalidTag):
		return Invalid(CodePostTagInvalid, "标签无效")
	case errors.Is(err, repository.ErrConflict):
		return Conflict(CodePostConflict, "帖子数据冲突")
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}
