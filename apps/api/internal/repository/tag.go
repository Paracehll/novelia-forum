package repository

import (
	"context"
	"database/sql"
	"time"

	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	"forum/internal/domain"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
)

type Tag = domain.Tag

type TagRepository interface {
	ListByCategory(ctx context.Context, categoryID int64) ([]Tag, error)
	ListActive(ctx context.Context) ([]Tag, error)
	ListForPost(ctx context.Context, postID int64) ([]Tag, error)
	ListForPosts(ctx context.Context, postIDs []int64) (map[int64][]Tag, error)
	Create(
		ctx context.Context,
		categoryID int64,
		name string,
		color int16,
		sortOrder int32,
		attr string,
	) (*Tag, error)
	Update(ctx context.Context, categoryID, id int64, name string, color int16, sortOrder int32) (*Tag, error)
	SetActive(ctx context.Context, categoryID, id int64, active bool) error
}

type tagRepository struct{ db *sql.DB }

func NewTagRepository(db *sql.DB) TagRepository { return &tagRepository{db: db} }

func (r *tagRepository) ListByCategory(ctx context.Context, categoryID int64) ([]Tag, error) {
	stmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag).
		WHERE(table.Tag.CategoryID.EQ(Int64(categoryID))).
		ORDER_BY(table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	var dest []model.Tag
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.ListByCategory")
	}
	return tagsFromModels(dest), nil
}

func (r *tagRepository) ListActive(ctx context.Context) ([]Tag, error) {
	stmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag).
		WHERE(table.Tag.IsActive.IS_TRUE()).
		ORDER_BY(table.Tag.CategoryID.ASC(), table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	var dest []model.Tag
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.ListActive")
	}
	return tagsFromModels(dest), nil
}

func (r *tagRepository) ListForPost(ctx context.Context, postID int64) ([]Tag, error) {
	return listPostTags(ctx, queryDB(ctx, r.db), postID)
}

// listPostTags also accepts a transaction so writes can assemble their result
// before committing, without reading through a separate database connection.
func listPostTags(ctx context.Context, db qrm.DB, postID int64) ([]Tag, error) {
	stmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag.INNER_JOIN(table.PostTag, table.Tag.ID.EQ(table.PostTag.TagID))).
		WHERE(table.PostTag.PostID.EQ(Int64(postID))).
		ORDER_BY(table.Tag.SortOrder.ASC(), table.Tag.ID.ASC())
	var dest []model.Tag
	if err := stmt.QueryContext(ctx, db, &dest); err != nil {
		return nil, storageError(err, "tag.ListForPost")
	}
	return tagsFromModels(dest), nil
}

func (r *tagRepository) ListForPosts(ctx context.Context, postIDs []int64) (map[int64][]Tag, error) {
	dest := make(map[int64][]Tag, len(postIDs))
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
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &records); err != nil {
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
) (*Tag, error) {
	dest := model.Tag{CategoryID: categoryID, Name: name, Color: color, SortOrder: sortOrder, Attr: attr}
	stmt := table.Tag.INSERT(table.Tag.CategoryID, table.Tag.Name, table.Tag.Color, table.Tag.SortOrder, table.Tag.Attr).
		MODEL(dest).
		RETURNING(table.Tag.AllColumns)
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
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
) (*Tag, error) {
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
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return nil, storageError(err, "tag.Update")
	}
	tag := tagFromModel(dest)
	return &tag, nil
}

func (r *tagRepository) SetActive(ctx context.Context, categoryID, id int64, active bool) error {
	stmt := table.Tag.UPDATE(table.Tag.IsActive, table.Tag.UpdatedAt).
		SET(Bool(active), TimestampzT(time.Now())).
		WHERE(table.Tag.ID.EQ(Int64(id)).AND(table.Tag.CategoryID.EQ(Int64(categoryID))))
	result, err := stmt.ExecContext(ctx, queryDB(ctx, r.db))
	if err != nil {
		return storageError(err, "tag.SetActive")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return storageError(err, "tag.SetActive")
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func tagFromModel(tag model.Tag) Tag {
	return Tag{
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

func tagsFromModels(tags []model.Tag) []Tag {
	if tags == nil {
		return nil
	}
	dest := make([]Tag, len(tags))
	for i, tag := range tags {
		dest[i] = tagFromModel(tag)
	}
	return dest
}
