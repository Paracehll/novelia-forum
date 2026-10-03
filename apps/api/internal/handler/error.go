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
			return &httpx.HttpError{StatusCode: mapping.status, Message: mapping.err.Error(), Cause: err}
		}
	}
	return err
}

var commentErrors = []struct {
	err    error
	status int
}{
	{usecase.ErrCommentContentInvalid, http.StatusBadRequest},
	{usecase.ErrCommentNotOwner, http.StatusForbidden},
	{usecase.ErrCommentEditExpired, http.StatusForbidden},
	{usecase.ErrCommentNotFound, http.StatusNotFound},
	{usecase.ErrCommentsLocked, http.StatusConflict},
	{usecase.ErrCommentSubjectNotFound, http.StatusNotFound},
	{usecase.ErrCommentSubjectUnavailable, http.StatusServiceUnavailable},
	{usecase.ErrCommentRootNotFound, http.StatusNotFound},
	{usecase.ErrCommentDomainBlocked, http.StatusBadRequest},
	{usecase.ErrCommentDomainInvalid, http.StatusBadRequest},
}
