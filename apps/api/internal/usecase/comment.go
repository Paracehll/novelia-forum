package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"forum/internal/domain"
	"forum/internal/domainfilter"
	"forum/internal/repository"
)

const (
	CodeCommentAccountTooYoung    = "comment.account_too_young"
	CodeCommentContentInvalid     = "comment.content_invalid"
	CodeCommentNotOwner           = "comment.not_owner"
	CodeCommentAdminRequired      = "comment.admin_required"
	CodeCommentEditExpired        = "comment.edit_expired"
	CodeCommentNotFound           = "comment.not_found"
	CodeCommentNotEditable        = "comment.not_editable"
	CodeCommentStatusInvalid      = "comment.status_invalid"
	CodeCommentLocked             = "comment.locked"
	CodeCommentSubjectNotFound    = "comment.subject_not_found"
	CodeCommentSubjectTypeInvalid = "comment.subject_type_invalid"
	CodeCommentSubjectKeyInvalid  = "comment.subject_key_invalid"
	CodeCommentRootNotFound       = "comment.root_not_found"
	CodeCommentRootInvalid        = "comment.root_invalid"
	CodeCommentConflict           = "comment.conflict"
	CodeCommentDomainBlocked      = "comment.domain_blocked"
	CodeCommentDomainInvalid      = "comment.domain_invalid"
)

// SubjectResolver is the application-facing view of the external subject
// registry. Concrete plugins own their validation and availability checks.
type SubjectResolver interface {
	Type(kind string) (domain.CommentSubjectType, bool)
	Valid(kind, key string) bool
	Check(ctx context.Context, kind, key string) (bool, error)
}

// CommentPostRepository provides the post operations needed by comment workflows.
type CommentPostRepository interface {
	ExistsPublished(ctx context.Context, id int64) (bool, error)
	Lock(ctx context.Context, id int64) (*domain.Post, error)
	AdjustCommentsCount(ctx context.Context, id int64, delta int32, activeAt *time.Time) error
}

type CommentUsecase struct {
	tx              repository.TransactionRunner
	commentRepo     repository.CommentRepository
	postRepo        CommentPostRepository
	domainFilter    *domainfilter.Filter
	subjectResolver SubjectResolver
}

func NewCommentUsecase(
	tx repository.TransactionRunner,
	commentRepo repository.CommentRepository,
	postRepo CommentPostRepository,
	domainFilter *domainfilter.Filter,
	subjectResolver SubjectResolver,
) *CommentUsecase {
	if tx == nil {
		panic("comment usecase requires a transaction runner")
	}
	return &CommentUsecase{
		tx:              tx,
		commentRepo:     commentRepo,
		postRepo:        postRepo,
		domainFilter:    domainFilter,
		subjectResolver: subjectResolver,
	}
}

type ListAdminCommentsQuery struct {
	Search     string
	AuthorName string
	PostID     int64
	Status     *domain.CommentStatus
	Limit      int64
	Offset     int64
}

func checkCommentAdmin(actor Actor) error {
	if !actor.IsAdmin {
		return PermissionDenied(CodeCommentAdminRequired, "需要管理员权限")
	}
	return nil
}

func (u *CommentUsecase) ListAdmin(
	ctx context.Context,
	actor Actor,
	query ListAdminCommentsQuery,
) (int64, []domain.CommentReadModel, error) {
	if err := checkCommentAdmin(actor); err != nil {
		return 0, nil, err
	}
	if query.Status != nil && !query.Status.Valid() {
		return 0, nil, Invalid(CodeCommentStatusInvalid, "评论状态无效")
	}
	filter := repository.CommentFilter{
		Search:     query.Search,
		AuthorName: query.AuthorName,
		PostID:     query.PostID,
		Status:     query.Status,
	}
	total, items, err := u.commentRepo.ListAdmin(ctx, filter, query.Limit, query.Offset)
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_admin: %w", err)
	}
	if actor.UserID > 0 && len(items) > 0 {
		authorIDs := make([]int64, len(items))
		for i, comment := range items {
			authorIDs[i] = comment.AuthorID
		}
		blocked, err := u.commentRepo.BlockedAuthorIDs(ctx, actor.UserID, authorIDs)
		if err != nil {
			return 0, nil, fmt.Errorf("comment.list_author_blocks: %w", err)
		}
		for i := range items {
			items[i].AuthorBlocked = blocked[items[i].AuthorID]
		}
	}
	return total, items, nil
}

func (u *CommentUsecase) checkSubjectExist(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	key string,
) error {
	if subjectType != domain.CommentSubjectPost {
		return nil
	}
	id, err := domain.PostIDFromCommentSubjectKey(key)
	if err != nil {
		return fmt.Errorf("comment.check_post: %w", err)
	}
	exists, err := u.postRepo.ExistsPublished(ctx, id)
	if err != nil {
		return fmt.Errorf("comment.check_post: %w", err)
	}
	if !exists {
		return NotFound(CodeCommentSubjectNotFound, "评论所属帖子不存在")
	}
	return nil
}

type ListCommentsQuery struct {
	SubjectType domain.CommentSubjectType
	SubjectKey  string
	Limit       int64
	Offset      int64
}

func (u *CommentUsecase) List(
	ctx context.Context,
	actor Actor,
	query ListCommentsQuery,
) (int64, []domain.CommentThreadPreview, error) {
	if err := u.checkSubjectExist(ctx, query.SubjectType, query.SubjectKey); err != nil {
		return 0, nil, err
	}
	total, items, err := u.commentRepo.ListRoots(
		ctx,
		query.SubjectType, query.SubjectKey, query.Limit, query.Offset,
		actor.UserID,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_roots: %w", err)
	}
	if !actor.IsAdmin {
		for i := range items {
			redactUnpublishedContent(&items[i].Root.Comment)
			for j := range items[i].Replies {
				redactUnpublishedContent(&items[i].Replies[j].Comment)
			}
		}
	}
	return total, items, nil
}

type ListCommentRepliesQuery struct {
	SubjectType domain.CommentSubjectType
	SubjectKey  string
	RootID      int64
	Limit       int64
	Offset      int64
}

func (u *CommentUsecase) ListReplies(
	ctx context.Context,
	actor Actor,
	query ListCommentRepliesQuery,
) (int64, []domain.CommentReadModel, error) {
	if err := u.checkSubjectExist(ctx, query.SubjectType, query.SubjectKey); err != nil {
		return 0, nil, err
	}
	total, items, err := u.commentRepo.ListReplies(
		ctx,
		query.SubjectType, query.SubjectKey, query.RootID, query.Limit, query.Offset,
		actor.UserID,
	)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, nil, NotFound(CodeCommentRootNotFound, "根评论不存在")
	}
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_replies: %w", err)
	}
	if !actor.IsAdmin {
		for i := range items {
			redactUnpublishedContent(&items[i].Comment)
		}
	}
	return total, items, nil
}

func redactUnpublishedContent(comment *domain.Comment) {
	if comment.Status != domain.CommentStatusPublished {
		comment.Content = ""
	}
}

func (u *CommentUsecase) checkContent(content string) error {
	if strings.TrimSpace(content) == "" || utf8.RuneCountInString(content) > 1000 {
		return Invalid(CodeCommentContentInvalid, "评论内容不能为空且不能超过 1000 字")
	}
	if u.domainFilter == nil {
		return nil
	}
	switch err := u.domainFilter.Check(content); {
	case err == nil:
		return nil
	case errors.Is(err, domainfilter.ErrBlocked):
		return Invalid(CodeCommentDomainBlocked, "评论包含禁止使用的域名")
	case errors.Is(err, domainfilter.ErrText), errors.Is(err, domainfilter.ErrCandidate):
		return Invalid(CodeCommentDomainInvalid, "评论无法完成域名检查")
	default:
		return fmt.Errorf("comment.check_content: %w", err)
	}
}

func (u *CommentUsecase) findModifiableComment(
	ctx context.Context,
	actor Actor,
	subjectType domain.CommentSubjectType,
	id int64,
) (*domain.Comment, error) {
	comment, err := u.commentRepo.Lock(ctx, subjectType, id)
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrNotFound):
		return nil, NotFound(CodeCommentNotFound, "评论不存在")
	default:
		return nil, fmt.Errorf("comment.find: %w", err)
	}
	if comment.AuthorID != actor.UserID && !actor.IsAdmin {
		return nil, PermissionDenied(CodeCommentNotOwner, "只能修改自己的评论")
	}
	if !actor.IsAdmin && time.Now().After(comment.CreatedAt.Add(20*time.Minute)) {
		return nil, PermissionDenied(CodeCommentEditExpired, "评论只能在发布后 20 分钟内编辑或删除")
	}
	return comment, nil
}

func checkCommentPublisher(actor Actor) error {
	if actor.CreatedAt.IsZero() || time.Now().Before(actor.CreatedAt.Add(30*24*time.Hour)) {
		return PermissionDenied(CodeCommentAccountTooYoung, "注册满 30 天后才能发表评论")
	}
	return nil
}

type CreatePostCommentCommand struct {
	PostID  int64
	RootID  *int64
	Content string
}

func (u *CommentUsecase) Create(
	ctx context.Context,
	actor Actor,
	command CreatePostCommentCommand,
) (*domain.CommentReadModel, error) {
	if err := checkCommentPublisher(actor); err != nil {
		return nil, err
	}
	if err := u.checkContent(command.Content); err != nil {
		return nil, err
	}
	return u.createComment(ctx, actor, domain.Comment{
		SubjectType: domain.CommentSubjectPost,
		SubjectKey:  domain.PostCommentSubjectKey(command.PostID),
		RootID:      command.RootID,
		Content:     command.Content,
	}, "comment.create_post")
}

type CreateExternalCommentCommand struct {
	Kind       string
	SubjectKey string
	RootID     *int64
	Content    string
}

func (u *CommentUsecase) CreateExternal(
	ctx context.Context,
	actor Actor,
	command CreateExternalCommentCommand,
) (*domain.CommentReadModel, error) {
	if err := checkCommentPublisher(actor); err != nil {
		return nil, err
	}
	if err := u.checkContent(command.Content); err != nil {
		return nil, err
	}
	subjectType, err := u.externalSubjectType(command.Kind)
	if err != nil {
		return nil, err
	}
	if !u.subjectResolver.Valid(command.Kind, command.SubjectKey) {
		return nil, Invalid(CodeCommentSubjectKeyInvalid, "subjectKey 格式无效")
	}
	exists, err := u.subjectResolver.Check(ctx, command.Kind, command.SubjectKey)
	if err != nil {
		return nil, fmt.Errorf("comment.check_subject: %w", err)
	}
	if !exists {
		return nil, NotFound(CodeCommentSubjectNotFound, "评论所属资源不存在")
	}

	return u.createComment(ctx, actor, domain.Comment{
		SubjectType: subjectType,
		SubjectKey:  command.SubjectKey,
		RootID:      command.RootID,
		Content:     command.Content,
	}, "comment.create_external")
}

func (u *CommentUsecase) createComment(
	ctx context.Context,
	actor Actor,
	input domain.Comment,
	operation string,
) (*domain.CommentReadModel, error) {
	input.AuthorID = actor.UserID
	input.AuthorUsername = actor.Username
	input.Status = domain.CommentStatusPublished
	var comment *domain.Comment
	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var postID int64
		if input.SubjectType == domain.CommentSubjectPost {
			var err error
			postID, err = domain.PostIDFromCommentSubjectKey(input.SubjectKey)
			if err != nil {
				return Invalid(CodeCommentSubjectKeyInvalid, "评论所属帖子 ID 无效")
			}
			post, err := u.postRepo.Lock(txCtx, postID)
			if errors.Is(err, repository.ErrNotFound) {
				return NotFound(CodeCommentSubjectNotFound, "评论所属资源不存在")
			}
			if err != nil {
				return fmt.Errorf("comment.lock_post: %w", err)
			}
			if post.Status != domain.PostStatusPublished {
				return NotFound(CodeCommentSubjectNotFound, "评论所属资源不存在")
			}
			if post.CommentsLocked {
				return Conflict(CodeCommentLocked, "评论区已锁定")
			}
		}
		if input.RootID != nil {
			// Keep this a non-locking read: creation locks the post first,
			// whereas moderation locks the comment before updating its post.
			root, err := u.commentRepo.Find(txCtx, input.SubjectType, *input.RootID)
			if errors.Is(err, repository.ErrNotFound) {
				return NotFound(CodeCommentRootNotFound, "根评论不存在")
			}
			if err != nil {
				return fmt.Errorf("comment.find_root: %w", err)
			}
			if root.SubjectType != input.SubjectType || root.Status != domain.CommentStatusPublished {
				return NotFound(CodeCommentRootNotFound, "根评论不存在")
			}
			if root.SubjectKey != input.SubjectKey || root.RootID != nil {
				return Invalid(CodeCommentRootInvalid, "根评论无效")
			}
		}
		var err error
		comment, err = u.commentRepo.Create(txCtx, input)
		if errors.Is(err, repository.ErrConflict) {
			return Conflict(CodeCommentConflict, "评论数据冲突")
		}
		if err != nil {
			return fmt.Errorf("comment.insert: %w", err)
		}
		if input.SubjectType == domain.CommentSubjectPost {
			now := time.Now()
			if err := u.postRepo.AdjustCommentsCount(txCtx, postID, 1, &now); err != nil {
				return fmt.Errorf("comment.increment_count: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return &domain.CommentReadModel{Comment: *comment}, nil
}

type UpdateCommentCommand struct {
	SubjectType domain.CommentSubjectType
	CommentID   int64
	Content     string
}

func (u *CommentUsecase) Update(
	ctx context.Context,
	actor Actor,
	command UpdateCommentCommand,
) (*domain.CommentReadModel, error) {
	if err := u.checkContent(command.Content); err != nil {
		return nil, err
	}
	var comment *domain.Comment
	authorBlocked := false
	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := u.findModifiableComment(txCtx, actor, command.SubjectType, command.CommentID)
		if err != nil {
			return err
		}
		if actor.UserID > 0 && actor.UserID != existing.AuthorID {
			blocked, err := u.commentRepo.BlockedAuthorIDs(txCtx, actor.UserID, []int64{existing.AuthorID})
			if err != nil {
				return fmt.Errorf("comment.get_author_block: %w", err)
			}
			authorBlocked = blocked[existing.AuthorID]
		}
		if !existing.CanEditContent() {
			return Conflict(CodeCommentNotEditable, "只有已发布的评论可以编辑")
		}
		comment, err = u.commentRepo.Update(txCtx, command.SubjectType, command.CommentID, command.Content)
		if errors.Is(err, repository.ErrNotFound) {
			return Conflict(CodeCommentConflict, "评论数据已变化，请刷新后重试")
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("comment.update: %w", err)
	}
	return &domain.CommentReadModel{Comment: *comment, AuthorBlocked: authorBlocked}, nil
}

type DeleteCommentCommand struct {
	SubjectType domain.CommentSubjectType
	CommentID   int64
}

func (u *CommentUsecase) Delete(ctx context.Context, actor Actor, command DeleteCommentCommand) error {
	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := u.findModifiableComment(txCtx, actor, command.SubjectType, command.CommentID)
		if err != nil {
			return err
		}
		return u.applyStatus(txCtx, existing, domain.CommentStatusDeleted)
	})
	if err != nil {
		return fmt.Errorf("comment.delete: %w", err)
	}
	return nil
}

type SetCommentStatusCommand struct {
	SubjectType domain.CommentSubjectType
	CommentID   int64
	Status      domain.CommentStatus
}

func (u *CommentUsecase) SetStatus(ctx context.Context, actor Actor, command SetCommentStatusCommand) error {
	if err := checkCommentAdmin(actor); err != nil {
		return err
	}
	if !command.Status.Valid() {
		return Invalid(CodeCommentStatusInvalid, "评论状态无效")
	}
	return u.setStatus(ctx, command, "comment.set_status")
}

func (u *CommentUsecase) setStatus(ctx context.Context, command SetCommentStatusCommand, operation string) error {
	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := u.commentRepo.Lock(txCtx, command.SubjectType, command.CommentID)
		if errors.Is(err, repository.ErrNotFound) {
			return NotFound(CodeCommentNotFound, "评论不存在")
		}
		if err != nil {
			return fmt.Errorf("comment.lock: %w", err)
		}
		return u.applyStatus(txCtx, existing, command.Status)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// applyStatus uses the caller's transaction and locked pre-change comment.
func (u *CommentUsecase) applyStatus(ctx context.Context, existing *domain.Comment, status domain.CommentStatus) error {
	if err := u.commentRepo.SetStatus(ctx, existing.SubjectType, existing.ID, status); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return NotFound(CodeCommentNotFound, "评论不存在")
		}
		return fmt.Errorf("comment.write_status: %w", err)
	}
	if existing.SubjectType != domain.CommentSubjectPost || existing.Status == status {
		return nil
	}
	var delta int32
	var activeAt *time.Time
	if existing.Status == domain.CommentStatusPublished {
		delta = -1
	} else if status == domain.CommentStatusPublished {
		delta = 1
		now := time.Now()
		activeAt = &now
	} else {
		return nil
	}
	postID, err := domain.PostIDFromCommentSubjectKey(existing.SubjectKey)
	if err != nil {
		return fmt.Errorf("comment.status_post_id: %w", err)
	}
	if err := u.postRepo.AdjustCommentsCount(ctx, postID, delta, activeAt); err != nil {
		return fmt.Errorf("comment.adjust_count: %w", err)
	}
	return nil
}

type DeleteCommentsByAuthorCommand struct {
	AuthorID int64
}

func (u *CommentUsecase) DeleteAllByAuthor(
	ctx context.Context,
	actor Actor,
	command DeleteCommentsByAuthorCommand,
) error {
	if err := checkCommentAdmin(actor); err != nil {
		return err
	}
	err := u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		comments, err := u.commentRepo.SetStatusByAuthor(txCtx, command.AuthorID,
			[]domain.CommentStatus{domain.CommentStatusPublished, domain.CommentStatusHidden},
			domain.CommentStatusDeleted)
		if err != nil {
			return fmt.Errorf("comment.delete_author_statuses: %w", err)
		}
		counts := make(map[int64]int32)
		for _, comment := range comments {
			if comment.SubjectType != domain.CommentSubjectPost || comment.Status != domain.CommentStatusPublished {
				continue
			}
			postID, err := domain.PostIDFromCommentSubjectKey(comment.SubjectKey)
			if err != nil {
				return fmt.Errorf("comment.delete_author_post_id: %w", err)
			}
			counts[postID]++
		}
		postIDs := make([]int64, 0, len(counts))
		for postID := range counts {
			postIDs = append(postIDs, postID)
		}
		// Multiple bulk operations must acquire post locks in the same order.
		sort.Slice(postIDs, func(i, j int) bool { return postIDs[i] < postIDs[j] })
		for _, postID := range postIDs {
			if err := u.postRepo.AdjustCommentsCount(txCtx, postID, -counts[postID], nil); err != nil {
				return fmt.Errorf("comment.delete_author_count: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("comment.delete_by_author: %w", err)
	}
	return nil
}

func (u *CommentUsecase) externalSubjectType(kind string) (domain.CommentSubjectType, error) {
	if u.subjectResolver == nil {
		return 0, Invalid(CodeCommentSubjectTypeInvalid, "不支持的外部资源类型")
	}
	subjectType, ok := u.subjectResolver.Type(kind)
	if !ok {
		return 0, Invalid(CodeCommentSubjectTypeInvalid, "不支持的外部资源类型")
	}
	return subjectType, nil
}

type ListExternalCommentsQuery struct {
	Kind       string
	SubjectKey string
	Limit      int64
	Offset     int64
}

func (u *CommentUsecase) ListExternal(
	ctx context.Context,
	actor Actor,
	query ListExternalCommentsQuery,
) (int64, []domain.CommentThreadPreview, error) {
	subjectType, err := u.externalSubjectType(query.Kind)
	if err != nil {
		return 0, nil, err
	}
	return u.List(ctx, actor, ListCommentsQuery{
		SubjectType: subjectType,
		SubjectKey:  query.SubjectKey,
		Limit:       query.Limit,
		Offset:      query.Offset,
	})
}

type ListExternalCommentRepliesQuery struct {
	Kind       string
	SubjectKey string
	RootID     int64
	Limit      int64
	Offset     int64
}

func (u *CommentUsecase) ListExternalReplies(
	ctx context.Context,
	actor Actor,
	query ListExternalCommentRepliesQuery,
) (int64, []domain.CommentReadModel, error) {
	subjectType, err := u.externalSubjectType(query.Kind)
	if err != nil {
		return 0, nil, err
	}
	return u.ListReplies(ctx, actor, ListCommentRepliesQuery{
		SubjectType: subjectType,
		SubjectKey:  query.SubjectKey,
		RootID:      query.RootID,
		Limit:       query.Limit,
		Offset:      query.Offset,
	})
}

type UpdateExternalCommentCommand struct {
	Kind      string
	CommentID int64
	Content   string
}

func (u *CommentUsecase) UpdateExternal(
	ctx context.Context,
	actor Actor,
	command UpdateExternalCommentCommand,
) (*domain.CommentReadModel, error) {
	subjectType, err := u.externalSubjectType(command.Kind)
	if err != nil {
		return nil, err
	}
	return u.Update(ctx, actor, UpdateCommentCommand{
		SubjectType: subjectType,
		CommentID:   command.CommentID,
		Content:     command.Content,
	})
}

type DeleteExternalCommentCommand struct {
	Kind      string
	CommentID int64
}

func (u *CommentUsecase) DeleteExternal(
	ctx context.Context,
	actor Actor,
	command DeleteExternalCommentCommand,
) error {
	subjectType, err := u.externalSubjectType(command.Kind)
	if err != nil {
		return err
	}
	return u.Delete(ctx, actor, DeleteCommentCommand{
		SubjectType: subjectType,
		CommentID:   command.CommentID,
	})
}

type SetExternalCommentStatusCommand struct {
	Kind      string
	CommentID int64
	Status    domain.CommentStatus
}

func (u *CommentUsecase) SetExternalStatus(
	ctx context.Context,
	actor Actor,
	command SetExternalCommentStatusCommand,
) error {
	if err := checkCommentAdmin(actor); err != nil {
		return err
	}
	subjectType, err := u.externalSubjectType(command.Kind)
	if err != nil {
		return err
	}
	return u.SetStatus(ctx, actor, SetCommentStatusCommand{
		SubjectType: subjectType,
		CommentID:   command.CommentID,
		Status:      command.Status,
	})
}
