package repository

import (
	"context"
	"database/sql"
	"time"

	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	"forum/internal/domain"

	. "github.com/go-jet/jet/v2/postgres"
)

type TagRepository interface {
	LockByIDs(ctx context.Context, ids []int64) ([]domain.Tag, error)
	ListByCategory(ctx context.Context, categoryID int64) ([]domain.Tag, error)
	ListActive(ctx context.Context) ([]domain.Tag, error)
	ListForPost(ctx context.Context, postID int64) ([]domain.Tag, error)
	ListForPosts(ctx context.Context, postIDs []int64) (map[int64][]domain.Tag, error)
	Create(
		ctx context.Context,
		categoryID int64,
		name string,
		color int16,
		sortOrder int32,
		attr string,
	) (*domain.Tag, error)
	Update(ctx context.Context, categoryID, id int64, name string, color int16, sortOrder int32) (*domain.Tag, error)
	SetActive(ctx context.Context, categoryID, id int64, active bool) error
}

type tagRepository struct{ db *sql.DB }

func NewTagRepository(db *sql.DB) TagRepository { return &tagRepository{db: db} }

// LockByIDs reads the selected tags without category or availability rules.
// Shared row locks keep their metadata stable until the usecase commits.
func (r *tagRepository) LockByIDs(ctx context.Context, ids []int64) ([]domain.Tag, error) {
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, storageError(err, "tag.LockByIDs")
	}
	if len(ids) == 0 {
		return []domain.Tag{}, nil
	}
	stmt := SELECT(table.Tag.AllColumns).FROM(table.Tag).
		WHERE(table.Tag.ID.IN(integerExpressions(ids)...)).
		ORDER_BY(table.Tag.SortOrder.ASC(), table.Tag.ID.ASC()).FOR(SHARE())
	var records []model.Tag
	if err := stmt.QueryContext(ctx, tx, &records); err != nil {
		return nil, storageError(err, "tag.LockByIDs")
	}
	return tagsFromModels(records), nil
}

func (r *tagRepository) ListByCategory(ctx context.Context, categoryID int64) ([]domain.Tag, error) {
	stmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag).
		WHERE(table.Tag.CategoryID.EQ(Int64(categoryID))).
		ORDER_BY(table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	var dest []model.Tag
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.ListByCategory")
	}
	return tagsFromModels(dest), nil
}

func (r *tagRepository) ListActive(ctx context.Context) ([]domain.Tag, error) {
	stmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag).
		WHERE(table.Tag.IsActive.IS_TRUE()).
		ORDER_BY(table.Tag.CategoryID.ASC(), table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	var dest []model.Tag
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.ListActive")
	}
	return tagsFromModels(dest), nil
}

func (r *tagRepository) ListForPost(ctx context.Context, postID int64) ([]domain.Tag, error) {
	stmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag.INNER_JOIN(table.PostTag, table.Tag.ID.EQ(table.PostTag.TagID))).
		WHERE(table.PostTag.PostID.EQ(Int64(postID))).
		ORDER_BY(table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	var dest []model.Tag
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.ListForPost")
	}
	return tagsFromModels(dest), nil
}

func (r *tagRepository) ListForPosts(ctx context.Context, postIDs []int64) (map[int64][]domain.Tag, error) {
	dest := make(map[int64][]domain.Tag, len(postIDs))
	if len(postIDs) == 0 {
		return dest, nil
	}

	var records []struct {
		PostID int64
		model.Tag
	}
	stmt := SELECT(table.PostTag.PostID.AS("PostID"), table.Tag.AllColumns).
		FROM(table.PostTag.INNER_JOIN(table.Tag, table.PostTag.TagID.EQ(table.Tag.ID))).
		WHERE(table.PostTag.PostID.IN(integerExpressions(postIDs)...)).
		ORDER_BY(table.PostTag.PostID.ASC(), table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &records); err != nil {
		return nil, storageError(err, "tag.ListForPosts")
	}
	for _, record := range records {
		dest[record.PostID] = append(dest[record.PostID], tagFromModel(record.Tag))
	}
	return dest, nil
}

func (r *tagRepository) Create(
	ctx context.Context,
	categoryID int64,
	name string,
	color int16,
	sortOrder int32,
	attr string,
) (*domain.Tag, error) {
	dest := model.Tag{CategoryID: categoryID, Name: name, Color: color, SortOrder: sortOrder, Attr: attr}
	stmt := table.Tag.INSERT(table.Tag.CategoryID, table.Tag.Name, table.Tag.Color, table.Tag.SortOrder, table.Tag.Attr).
		MODEL(dest).
		RETURNING(table.Tag.AllColumns)
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.Create")
	}
	tag := tagFromModel(dest)
	return &tag, nil
}

func (r *tagRepository) Update(
	ctx context.Context,
	categoryID, id int64,
	name string,
	color int16,
	sortOrder int32,
) (*domain.Tag, error) {
	stmt := table.Tag.UPDATE(
		table.Tag.Name,
		table.Tag.Color,
		table.Tag.SortOrder,
		table.Tag.UpdatedAt,
	).
		SET(String(name), Int16(color), Int32(sortOrder), TimestampzT(time.Now())).
		WHERE(table.Tag.ID.EQ(Int64(id)).AND(table.Tag.CategoryID.EQ(Int64(categoryID)))).
		RETURNING(table.Tag.AllColumns)
	var dest model.Tag
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.Update")
	}
	tag := tagFromModel(dest)
	return &tag, nil
}

func (r *tagRepository) SetActive(ctx context.Context, categoryID, id int64, active bool) error {
	stmt := table.Tag.UPDATE(table.Tag.IsActive, table.Tag.UpdatedAt).
		SET(Bool(active), TimestampzT(time.Now())).
		WHERE(table.Tag.ID.EQ(Int64(id)).AND(table.Tag.CategoryID.EQ(Int64(categoryID))))
	err := requireAffectedRow(stmt.ExecContext(ctx, executor(ctx, r.db)))
	return storageError(err, "tag.SetActive")
}

func tagFromModel(tag model.Tag) domain.Tag {
	return domain.Tag{
		ID:         tag.ID,
		CategoryID: tag.CategoryID,
		Name:       tag.Name,
		Color:      tag.Color,
		IsActive:   tag.IsActive,
		SortOrder:  tag.SortOrder,
		CreatedAt:  tag.CreatedAt,
		UpdatedAt:  tag.UpdatedAt,
		Attr:       tag.Attr,
	}
}

func tagsFromModels(tags []model.Tag) []domain.Tag {
	if tags == nil {
		return nil
	}
	dest := make([]domain.Tag, len(tags))
	for i, tag := range tags {
		dest[i] = tagFromModel(tag)
	}
	return dest
}
