package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"auth/internal/httpx"
	"auth/internal/subject"
	"auth/internal/usecase"
)

func TestCommentErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		err     error
		status  int
		message string
	}{
		{subject.ErrUnsupported, http.StatusBadRequest, "不支持的外部资源类型"},
		{subject.ErrInvalid, http.StatusBadRequest, "subjectKey 格式无效"},
		{usecase.ErrCommentContentInvalid, http.StatusBadRequest, "content 不能为空且不能超过 1000 字"},
		{usecase.ErrCommentDomainBlocked, http.StatusBadRequest, "内容包含禁止使用的域名"},
		{usecase.ErrCommentNotOwner, http.StatusForbidden, "只能修改自己的评论"},
		{usecase.ErrCommentEditExpired, http.StatusForbidden, "评论只能在发布后 20 分钟内编辑或删除"},
		{usecase.ErrCommentNotFound, http.StatusNotFound, "评论不存在"},
		{usecase.ErrCommentSubjectNotFound, http.StatusNotFound, "评论所属资源不存在"},
		{usecase.ErrCommentRootNotFound, http.StatusNotFound, "根评论不存在"},
		{usecase.ErrCommentDomainInvalid, http.StatusBadRequest, "内容无法完成域名检查"},
		{usecase.ErrCommentsLocked, http.StatusConflict, "评论区已锁定"},
		{usecase.ErrCommentSubjectUnavailable, http.StatusServiceUnavailable, "暂时无法校验资源，请稍后重试"},
		{fmt.Errorf("comment.list_roots: %w", errors.New("database unavailable")), http.StatusInternalServerError, "服务器内部错误"},
		{errors.New("评论不存在"), http.StatusInternalServerError, "服务器内部错误"},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			cause := errors.New("private database or upstream details")
			err := fmt.Errorf("operation failed: %w: %w", tc.err, cause)
			if !errors.Is(transportError(err), cause) || !errors.Is(transportError(err), tc.err) {
				t.Fatal("transport mapping lost original cause or sentinel")
			}
			response := httptest.NewRecorder()
			response.Header().Set("Content-Length", "999")
			httpx.EH(func(http.ResponseWriter, *http.Request) error { return transportError(err) })(
				response, httptest.NewRequest(http.MethodGet, "/", nil),
			)
			if response.Code != tc.status || response.Body.String() != tc.message {
				t.Fatalf("got (%d, %q), want (%d, %q)", response.Code, response.Body.String(), tc.status, tc.message)
			}
			if response.Header().Get("Content-Length") != "" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("unexpected error headers: %v", response.Header())
			}
		})
	}
}
