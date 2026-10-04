package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/repository"
)

const (
	CodeTagIDInvalid        = "tag.id_invalid"
	CodeTagCategoryInvalid  = "tag.category_invalid"
	CodeTagCategoryNotFound = "tag.category_not_found"
	CodeTagNameInvalid      = "tag.name_invalid"
	CodeTagColorInvalid     = "tag.color_invalid"
	CodeTagNotFound         = "tag.not_found"
	CodeTagConflict         = "tag.conflict"
	CodeTagAdminRequired    = "tag.admin_required"
)

type TagUsecase struct{ tagRepo repository.TagRepository }

func NewTagUsecase(tagRepo repository.TagRepository) *TagUsecase {
	return &TagUsecase{tagRepo: tagRepo}
}

type ListTagsQuery struct{ CategoryID int64 }
type TagInput struct {
	CategoryID int64
	Name       string
	Color      int16
	SortOrder  int32
}

func (u *TagUsecase) ListActive(ctx context.Context) ([]domain.Tag, error) {
	tags, err := u.tagRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("tag.list_active: %w", err)
	}
	return tags, nil
}

func checkTagAdmin(actor Actor) error {
	if !actor.IsAdmin {
		return PermissionDenied(CodeTagAdminRequired, "需要管理员权限")
	}
	return nil
}

func checkTagCategory(id int64) error {
	if id <= 0 {
		return Invalid(CodeTagCategoryInvalid, "categoryId 必须为正整数")
	}
	if _, ok := forumcategory.FindByID(id); !ok {
		return NotFound(CodeTagCategoryNotFound, "分类不存在")
	}
	return nil
}

func checkTagID(id int64) error {
	if id <= 0 {
		return Invalid(CodeTagIDInvalid, "标签 ID 必须为正整数")
	}
	return nil
}

func normalizeTagInput(input TagInput) (TagInput, error) {
	if err := checkTagCategory(input.CategoryID); err != nil {
		return input, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if n := utf8.RuneCountInString(input.Name); n == 0 || n > 64 {
		return input, Invalid(CodeTagNameInvalid, "name 不能为空且不能超过 64 字")
	}
	if input.Color < 0 {
		return input, Invalid(CodeTagColorInvalid, "color 不能为负数")
	}
	return input, nil
}

func tagError(err error, operation string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return NotFound(CodeTagNotFound, "标签不存在")
	}
	if errors.Is(err, repository.ErrConflict) {
		return Conflict(CodeTagConflict, "标签已存在")
	}
	if err != nil {
		return fmt.Errorf("tag.%s: %w", operation, err)
	}
	return nil
}

func (u *TagUsecase) ListAdmin(ctx context.Context, actor Actor, query ListTagsQuery) ([]domain.Tag, error) {
	if err := checkTagAdmin(actor); err != nil {
		return nil, err
	}
	if err := checkTagCategory(query.CategoryID); err != nil {
		return nil, err
	}
	tags, err := u.tagRepo.ListByCategory(ctx, query.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("tag.list: %w", err)
	}
	return tags, nil
}

func (u *TagUsecase) Create(ctx context.Context, actor Actor, input TagInput) (*domain.Tag, error) {
	if err := checkTagAdmin(actor); err != nil {
		return nil, err
	}
	input, err := normalizeTagInput(input)
	if err != nil {
		return nil, err
	}
	tag, err := u.tagRepo.Create(ctx, input.CategoryID, input.Name, input.Color, input.SortOrder, "{}")
	if err != nil {
		return nil, tagError(err, "create")
	}
	return tag, nil
}

func (u *TagUsecase) Update(ctx context.Context, actor Actor, id int64, input TagInput) (*domain.Tag, error) {
	if err := checkTagAdmin(actor); err != nil {
		return nil, err
	}
	if err := checkTagID(id); err != nil {
		return nil, err
	}
	input, err := normalizeTagInput(input)
	if err != nil {
		return nil, err
	}
	tag, err := u.tagRepo.Update(ctx, input.CategoryID, id, input.Name, input.Color, input.SortOrder)
	if err != nil {
		return nil, tagError(err, "update")
	}
	return tag, nil
}

type SetTagActiveCommand struct {
	CategoryID, ID int64
	Active         bool
}

func (u *TagUsecase) SetActive(ctx context.Context, actor Actor, command SetTagActiveCommand) error {
	if err := checkTagAdmin(actor); err != nil {
		return err
	}
	if err := checkTagCategory(command.CategoryID); err != nil {
		return err
	}
	if err := checkTagID(command.ID); err != nil {
		return err
	}
	return tagError(u.tagRepo.SetActive(ctx, command.CategoryID, command.ID, command.Active), "set_active")
}
