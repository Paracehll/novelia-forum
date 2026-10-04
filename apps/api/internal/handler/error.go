package handler

import (
	"errors"
	"net/http"

	"auth/internal/httpx"
	"auth/internal/subject"
	"auth/internal/usecase"
)

func transportError(err error) error {
	var httpErr *httpx.HttpError
	if errors.As(err, &httpErr) {
		return err
	}
	switch {
	case errors.Is(err, subject.ErrUnsupported):
		return &httpx.HttpError{StatusCode: http.StatusBadRequest, Message: "不支持的外部资源类型", Cause: err}
	case errors.Is(err, subject.ErrInvalid):
		return &httpx.HttpError{StatusCode: http.StatusBadRequest, Message: "subjectKey 格式无效", Cause: err}
	}
	for _, mapping := range commentErrors {
		if errors.Is(err, mapping.err) {
			return &httpx.HttpError{StatusCode: mapping.status, Message: mapping.message, Cause: err}
		}
	}
	return err
}

var commentErrors = []struct {
	err     error
	status  int
	message string
}{
	{usecase.ErrCommentContentInvalid, http.StatusBadRequest, "content 不能为空且不能超过 1000 字"},
	{usecase.ErrCommentNotOwner, http.StatusForbidden, "只能修改自己的评论"},
	{usecase.ErrCommentEditExpired, http.StatusForbidden, "评论只能在发布后 20 分钟内编辑或删除"},
	{usecase.ErrCommentNotFound, http.StatusNotFound, "评论不存在"},
	{usecase.ErrCommentsLocked, http.StatusConflict, "评论区已锁定"},
	{usecase.ErrCommentSubjectNotFound, http.StatusNotFound, "评论所属资源不存在"},
	{usecase.ErrCommentRootNotFound, http.StatusNotFound, "根评论不存在"},
	{usecase.ErrCommentDomainBlocked, http.StatusBadRequest, "内容包含禁止使用的域名"},
	{usecase.ErrCommentDomainInvalid, http.StatusBadRequest, "内容无法完成域名检查"},
	{usecase.ErrCommentRootInvalid, http.StatusBadRequest, "根评论无效"},
	{usecase.ErrCommentConflict, http.StatusConflict, "评论数据冲突"},
}
