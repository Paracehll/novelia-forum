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

const (
	CodeCommentContentInvalid     = "comment.content_invalid"
	CodeCommentNotOwner           = "comment.not_owner"
	CodeCommentEditExpired        = "comment.edit_expired"
	CodeCommentNotFound           = "comment.not_found"
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
		return Invalid(CodeCommentContentInvalid, "content 不能为空且不能超过 1000 字")
	}
	if u.domainFilter == nil {
		return nil
	}
	switch err := u.domainFilter.Check(content); {
	case err == nil:
		return nil
	case errors.Is(err, domainfilter.ErrBlocked):
		return Invalid(CodeCommentDomainBlocked, "内容包含禁止使用的域名")
	case errors.Is(err, domainfilter.ErrText), errors.Is(err, domainfilter.ErrCandidate):
		return Invalid(CodeCommentDomainInvalid, "内容无法完成域名检查")
	default:
		return fmt.Errorf("comment.check_content: %w", err)
	}
}

func (u *CommentUsecase) checkModifiable(principal Actor, subjectType int16, id int64) error {
	comment, err := u.commentRepo.Find(subjectType, id)
	if err != nil {
		return commentError(err, "comment.find")
	}
	if comment.AuthorID != principal.UserID && !principal.IsAdmin {
		return Forbidden(CodeCommentNotOwner, "只能修改自己的评论")
	}
	if !principal.IsAdmin && time.Now().After(comment.CreatedAt.Add(20*time.Minute)) {
		return Forbidden(CodeCommentEditExpired, "评论只能在发布后 20 分钟内编辑或删除")
	}
	return nil
}

func commentError(err error, operation string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodeCommentNotFound, "评论不存在")
	}
	if errors.Is(err, repository.ErrCommentRootNotFound) {
		return NotFound(CodeCommentRootNotFound, "根评论不存在")
	}
	if errors.Is(err, repository.ErrInvalidCommentRoot) {
		return Invalid(CodeCommentRootInvalid, "根评论无效")
	}
	if errors.Is(err, repository.ErrConflict) {
		return Conflict(CodeCommentConflict, "评论数据冲突")
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
		if errors.Is(err, repository.ErrNotFound) {
			return NotFound(CodeCommentSubjectNotFound, "评论所属资源不存在")
		}
		return fmt.Errorf("comment.check_post: %w", err)
	}
	return nil
}

func (u *CommentUsecase) ListRoots(
	principal Actor,
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
	if !principal.IsAdmin {
		for i := range items {
			if items[i].Status != repository.StatusPublished {
				items[i].Content = ""
			}
			for j := range items[i].Replies {
				if items[i].Replies[j].Status != repository.StatusPublished {
					items[i].Replies[j].Content = ""
				}
			}
		}
	}
	return total, items, nil
}

func (u *CommentUsecase) ListReplies(
	principal Actor,
	subjectType int16,
	key string,
	rootID, limit, offset int64,
) (int64, []repository.Comment, error) {
	if err := u.checkPost(subjectType, key); err != nil {
		return 0, nil, err
	}
	total, items, err := u.commentRepo.ListReplies(subjectType, key, rootID, limit, offset)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, nil, NotFound(CodeCommentRootNotFound, "根评论不存在")
	}
	if err != nil {
		return 0, nil, fmt.Errorf("comment.list_replies: %w", err)
	}
	if !principal.IsAdmin {
		for i := range items {
			if items[i].Status != repository.StatusPublished {
				items[i].Content = ""
			}
		}
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
	if errors.Is(err, repository.ErrNotFound) {
		return nil, NotFound(CodeCommentSubjectNotFound, "评论所属资源不存在")
	}
	if errors.Is(err, repository.ErrCommentsLocked) {
		return nil, Conflict(CodeCommentLocked, "评论区已锁定")
	}
	if err != nil {
		return nil, commentError(err, "comment.create_post")
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
			return nil, Invalid(CodeCommentSubjectTypeInvalid, "不支持的外部资源类型")
		case errors.Is(err, subject.ErrNotFound):
			return nil, NotFound(CodeCommentSubjectNotFound, "评论所属资源不存在")
		case errors.Is(err, subject.ErrInvalid):
			return nil, Invalid(CodeCommentSubjectKeyInvalid, "subjectKey 格式无效")
		default:
			return nil, fmt.Errorf("comment.check_subject: %w", err)
		}
	}

	input.SubjectType = subjectType
	input.Attr = "{}"
	comment, err := u.commentRepo.Create(input)
	if err != nil {
		return nil, commentError(err, "comment.create_external")
	}
	return comment, nil
}
