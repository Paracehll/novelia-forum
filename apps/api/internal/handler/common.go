package handler

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"unicode/utf8"

	"forum/internal/domainfilter"
	"forum/internal/httpx"
	"forum/internal/repository"
)

type page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
}

type pagination struct {
	Limit  int64
	Offset int64
}

func parsePagination(query url.Values, defaultPageSize, maxPageSize int64) (pagination, error) {
	page, err := httpx.ParseQueryPositiveInt(query, "page", 1)
	if err != nil {
		return pagination{}, err
	}
	pageSize, err := httpx.ParseQueryPositiveInt(query, "page_size", defaultPageSize)
	if err != nil {
		return pagination{}, err
	}
	if pageSize > maxPageSize {
		return pagination{}, httpx.BadRequest(fmt.Sprintf("page_size 不能超过 %d", maxPageSize))
	}
	if page-1 > math.MaxInt64/pageSize {
		return pagination{}, httpx.BadRequest("page 超出可支持的范围")
	}
	return pagination{Limit: pageSize, Offset: (page - 1) * pageSize}, nil
}

func repoError(err error, message string) error {
	if repository.IsNotFound(err) {
		return httpx.NotFound("资源不存在")
	}
	if errors.Is(err, repository.ErrInvalidCategory) {
		return httpx.BadRequest("分类无效")
	}
	if errors.Is(err, repository.ErrInvalidTag) {
		return httpx.BadRequest("标签无效")
	}
	if repository.IsUniqueViolation(err) {
		return httpx.Conflict("资源已存在")
	}
	return httpx.InternalError(err, message)
}

func validText(value string, min, max int) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= min && length <= max
}

func uniquePositiveIDs(ids []int64) bool {
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func validStatus(status int16) bool {
	return status >= repository.StatusPublished && status <= repository.StatusDeleted
}

func checkDomainText(filter *domainfilter.Filter, field, value string) error {
	if filter == nil {
		return nil
	}
	switch err := filter.Check(value); {
	case err == nil:
		return nil
	case errors.Is(err, domainfilter.ErrBlocked):
		return httpx.BadRequest(field + " 包含禁止使用的域名")
	case errors.Is(err, domainfilter.ErrText), errors.Is(err, domainfilter.ErrCandidate):
		return httpx.BadRequest(field + " 无法完成域名检查")
	default:
		return httpx.InternalError(err, "域名检查失败")
	}
}
