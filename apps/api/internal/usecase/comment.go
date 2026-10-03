package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"auth/internal/domainfilter"
	"auth/internal/repository"
	"auth/internal/subject"
)

var (
	ErrCommentContentInvalid     = errors.New("content 不能为空且不能超过 1000 字")
	ErrCommentNotOwner           = errors.New("只能修改自己的评论")
	ErrCommentEditExpired        = errors.New("评论只能在发布后 20 分钟内编辑或删除")
	ErrCommentNotFound           = errors.New("评论不存在")
	ErrCommentsLocked            = errors.New("评论区已锁定")
	ErrCommentSubjectNotFound    = errors.New("评论所属资源不存在")
	ErrCommentSubjectUnavailable = errors.New("暂时无法校验资源，请稍后重试")
	ErrCommentRootNotFound       = errors.New("根评论不存在")
	ErrCommentDomainBlocked      = errors.New("内容包含禁止使用的域名")
	ErrCommentDomainInvalid      = errors.New("内容无法完成域名检查")
)

// CommentUsecase shares comment rules across forum posts and external resources.
// Authentication and route role checks remain in transport middleware.
// Repository methods retain the transactions that maintain post comment counts.
type CommentUsecase struct {
	commentRepo    repository.CommentRepository
	postRepo       repository.PostRepository
	domainFilter   *domainfilter.Filter
	subjectChecker subject.Checker
}

func NewCommentUsecase(
	commentRepo repository.CommentRepository,
	postRepo repository.PostRepository,
	domainFilter *domainfilter.Filter,
	subjectChecker subject.Checker,
) *CommentUsecase {
	return &CommentUsecase{
		commentRepo:    commentRepo,
		postRepo:       postRepo,
		domainFilter:   domainFilter,
		subjectChecker: subjectChecker,
	}
}

func (u *CommentUsecase) checkContent(content string) error {
	if strings.TrimSpace(content) == "" || utf8.RuneCountInString(content) > 1000 {
		return ErrCommentContentInvalid
	}
	if u.domainFilter == nil {
		return nil
	}
	switch err := u.domainFilter.Check(content); {
	case err == nil:
		return nil
	case errors.Is(err, domainfilter.ErrBlocked):
		return ErrCommentDomainBlocked
	case errors.Is(err, domainfilter.ErrText), errors.Is(err, domainfilter.ErrCandidate):
		return ErrCommentDomainInvalid
	default:
		return fmt.Errorf("comment.check_content: %w", err)
	}
}

func CommentContent(principal Actor, value repository.Comment) string {
	if value.Status == repository.StatusPublished || principal.IsAdmin {
		return value.Content
	}
	return ""
}

func (u *CommentUsecase) checkModifiable(principal Actor, subjectType int16, id int64) error {
	comment, err := u.commentRepo.Find(subjectType, id)
	if err != nil {
		return commentError(err, "comment.find")
	}
	if comment.AuthorID != principal.UserID && !principal.IsAdmin {
		return ErrCommentNotOwner
	}
	if !principal.IsAdmin && time.Now().After(comment.CreatedAt.Add(20*time.Minute)) {
		return ErrCommentEditExpired
	}
	return nil
}

func commentError(err error, operation string) error {
	if err == nil {
		return nil
	}
	if repository.IsNotFound(err) {
		return ErrCommentNotFound
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (u *CommentUsecase) Update(
	principal Actor,
	subjectType int16,
	id int64,
	content string,
) (*repository.Comment, error) {
	if err := u.checkModifiable(principal, subjectType, id); err != nil {
		return nil, err
	}
	if err := u.checkContent(content); err != nil {
		return nil, err
	}
	comment, err := u.commentRepo.Update(subjectType, id, content)
	return comment, commentError(err, "comment.update")
}

func (u *CommentUsecase) Delete(principal Actor, subjectType int16, id int64) error {
	if err := u.checkModifiable(principal, subjectType, id); err != nil {
		return err
	}
	return commentError(u.commentRepo.SetStatus(subjectType, id, repository.StatusDeleted), "comment.delete")
}

func (u *CommentUsecase) SetStatus(subjectType int16, id int64, status int16) error {
	return commentError(u.commentRepo.SetStatus(subjectType, id, status), "comment.set_status")
}

func (u *CommentUsecase) DeleteAllByAuthor(authorID int64) error {
	if err := u.commentRepo.DeleteAllByAuthor(authorID); err != nil {
		return fmt.Errorf("comment.delete_by_author: %w", err)
	}
	return nil
}

func (u *CommentUsecase) ListAdmin(
	filter repository.CommentFilter,
	limit, offset int64,
) (int64, []repository.Comment, error) {
	total, items, err := u.commentRepo.ListAdmin(filter, limit, offset)
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_admin: %w", err)
	}
	return total, items, nil
}

func (u *CommentUsecase) checkPost(subjectType int16, key string) error {
	if subjectType != repository.CommentSubjectPost {
		return nil
	}
	id, err := repository.PostIDFromSubjectKey(key)
	if err != nil {
		return fmt.Errorf("comment.check_post: %w", err)
	}
	if _, err := u.postRepo.Find(id, false); err != nil {
		if repository.IsNotFound(err) {
			return ErrCommentSubjectNotFound
		}
		return fmt.Errorf("comment.check_post: %w", err)
	}
	return nil
}

func (u *CommentUsecase) ListRoots(
	subjectType int16,
	key string,
	limit, offset int64,
) (int64, []repository.CommentThread, error) {
	if err := u.checkPost(subjectType, key); err != nil {
		return 0, nil, err
	}
	total, items, err := u.commentRepo.ListRoots(subjectType, key, limit, offset)
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_roots: %w", err)
	}
	return total, items, nil
}

func (u *CommentUsecase) ListReplies(
	subjectType int16,
	key string,
	rootID, limit, offset int64,
) (int64, []repository.Comment, error) {
	if err := u.checkPost(subjectType, key); err != nil {
		return 0, nil, err
	}
	total, items, err := u.commentRepo.ListReplies(subjectType, key, rootID, limit, offset)
	if repository.IsNotFound(err) {
		return 0, nil, ErrCommentRootNotFound
	}
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_replies: %w", err)
	}
	return total, items, nil
}

func (u *CommentUsecase) CreatePost(input repository.CreateCommentInput) (*repository.Comment, error) {
	if err := u.checkContent(input.Content); err != nil {
		return nil, err
	}
	input.SubjectType = repository.CommentSubjectPost
	input.Attr = "{}"
	comment, err := u.commentRepo.Create(input)
	if repository.IsNotFound(err) {
		return nil, ErrCommentSubjectNotFound
	}
	if errors.Is(err, repository.ErrCommentsLocked) {
		return nil, ErrCommentsLocked
	}
	if err != nil {
		return nil, fmt.Errorf("comment.create_post: %w", err)
	}
	return comment, nil
}

func (u *CommentUsecase) CreateExternal(
	ctx context.Context,
	kind string,
	input repository.CreateCommentInput,
) (*repository.Comment, error) {
	if err := u.checkContent(input.Content); err != nil {
		return nil, err
	}
	subjectType, err := u.subjectChecker.Check(ctx, kind, input.SubjectKey)
	if err != nil {
		switch {
		case errors.Is(err, subject.ErrUnsupported):
			return nil, err
		case errors.Is(err, subject.ErrNotFound):
			return nil, ErrCommentSubjectNotFound
		case errors.Is(err, subject.ErrInvalid):
			return nil, err
		default:
			return nil, fmt.Errorf("%w: %w", ErrCommentSubjectUnavailable, err)
		}
	}

	input.SubjectType = subjectType
	input.Attr = "{}"
	comment, err := u.commentRepo.Create(input)
	if repository.IsNotFound(err) {
		return nil, ErrCommentRootNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("comment.create_external: %w", err)
	}
	return comment, nil
}
