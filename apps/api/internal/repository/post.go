package repository

import (
	"context"
	"database/sql"
	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
)

const (
	PostSortActive   = "active"
	PostSortNewest   = "newest"
	PostSortViews    = "views"
	PostSortComments = "comments"
	PostStatusAll    = -1
)

type PostFilter struct {
	CategorySlug, Search     string
	AuthorName               string
	Sort                     string
	TagIDs                   []int64
	AuthorID, FavoriteUserID int64
	Status                   domain.PostStatus
}

type CreatePostInput struct {
	CategoryID     int64
	Title, Content string
	AuthorID       int64
	AuthorUsername string
	TagIDs         []int64
	Attr           string
}

type UpdatePostInput struct {
	CategoryID     int64
	Title, Content string
	TagIDs         []int64
}

// Create and Update require a context provided by WithinTransaction.
type PostRepository interface {
	List(ctx context.Context, filter PostFilter, limit, offset int64) (int64, []domain.PostListItem, error)
	Find(ctx context.Context, id int64, incrementViews bool) (*domain.Post, error)
	ExistsPublished(ctx context.Context, id int64) (bool, error)
	Create(ctx context.Context, input CreatePostInput) (*domain.Post, error)
	Update(ctx context.Context, id int64, input UpdatePostInput) (*domain.Post, error)
	SetStatus(ctx context.Context, id int64, status domain.PostStatus) error
	SetCommentsLocked(ctx context.Context, id int64, locked bool) error
	SetPinOrder(ctx context.Context, id int64, pinOrder *int32) error
}

type postRepository struct {
	db      *sql.DB
	tagRepo TagRepository
}

func NewPostRepository(db *sql.DB, tagRepo TagRepository) PostRepository {
	return &postRepository{db: db, tagRepo: tagRepo}
}

// postFromModel keeps generated database models behind the repository boundary.
func postFromModel(record model.Post, tags []Tag) domain.Post {
	return domain.Post{
		ID: record.ID, CategoryID: record.CategoryID, Title: record.Title,
		AuthorID: record.AuthorID, AuthorUsername: record.AuthorUsername,
		Content: record.Content, Status: domain.PostStatus(record.Status),
		ViewsCount: record.ViewsCount, CommentsCount: record.CommentsCount,
		CommentsLocked: record.CommentsLocked, PinOrder: record.PinOrder,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ActiveAt: record.ActiveAt,
		Tags: postTagsFromModels(tags),
	}
}

func postListItemFromModel(record model.Post, tags []Tag) domain.PostListItem {
	return domain.PostListItem{
		ID: record.ID, CategoryID: record.CategoryID, Title: record.Title,
		AuthorID: record.AuthorID, AuthorUsername: record.AuthorUsername,
		Status:     domain.PostStatus(record.Status),
		ViewsCount: record.ViewsCount, CommentsCount: record.CommentsCount,
		CommentsLocked: record.CommentsLocked, PinOrder: record.PinOrder,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ActiveAt: record.ActiveAt,
		Tags: postTagsFromModels(tags),
	}
}

func postTagsFromModels(tags []Tag) []domain.PostTag {
	postTags := make([]domain.PostTag, len(tags))
	for i, tag := range tags {
		postTags[i] = domain.PostTag{ID: tag.ID, Name: tag.Name, Color: tag.Color}
	}
	return postTags
}

func integerExpressions(ids []int64) []Expression {
	expressions := make([]Expression, len(ids))
	for i, id := range ids {
		expressions[i] = Int64(id)
	}
	return expressions
}

func (filter PostFilter) condition() BoolExpression {
	expressions := []BoolExpression{RawBool("TRUE")}
	if filter.Status != PostStatusAll {
		expressions = append(expressions, table.Post.Status.EQ(Int16(int16(filter.Status))))
	}
	if filter.CategorySlug != "" {
		category, ok := forumcategory.FindBySlug(filter.CategorySlug)
		if !ok {
			expressions = append(expressions, RawBool("FALSE"))
		} else {
			expressions = append(expressions, table.Post.CategoryID.EQ(Int64(category.ID)))
		}
	}
	if filter.AuthorID > 0 {
		expressions = append(expressions, table.Post.AuthorID.EQ(Int64(filter.AuthorID)))
	}
	if filter.AuthorName != "" {
		expressions = append(expressions, RawBool(
			`"post"."author_username" ILIKE :authorPattern`,
			RawArgs{":authorPattern": "%" + filter.AuthorName + "%"},
		))
	}
	if filter.FavoriteUserID > 0 {
		expressions = append(expressions, table.PostFavorite.UserID.EQ(Int64(filter.FavoriteUserID)))
	}
	if filter.Search != "" {
		expressions = append(expressions, RawBool(
			`("post"."title" ILIKE :pattern OR "post"."content" ILIKE :pattern)`,
			RawArgs{":pattern": "%" + filter.Search + "%"},
		))
	}
	if len(filter.TagIDs) > 0 {
		taggedPosts := SELECT(table.PostTag.PostID).
			FROM(table.PostTag).
			WHERE(table.PostTag.TagID.IN(integerExpressions(filter.TagIDs)...)).
			GROUP_BY(table.PostTag.PostID).
			HAVING(COUNT(table.PostTag.TagID).EQ(Int64(int64(len(filter.TagIDs)))))
		expressions = append(expressions, table.Post.ID.IN(taggedPosts))
	}
	return AND(expressions...)
}

func postFrom(filter PostFilter) ReadableTable {
	if filter.FavoriteUserID > 0 {
		return table.Post.INNER_JOIN(table.PostFavorite, table.PostFavorite.PostID.EQ(table.Post.ID))
	}
	return table.Post
}

func postOrderBy(sort string) []OrderByClause {
	pinned := table.Post.PinOrder.ASC().NULLS_LAST()
	switch sort {
	case PostSortNewest:
		return []OrderByClause{pinned, table.Post.CreatedAt.DESC(), table.Post.ID.DESC()}
	case PostSortViews:
		return []OrderByClause{pinned, table.Post.ViewsCount.DESC(), table.Post.ActiveAt.DESC(), table.Post.ID.DESC()}
	case PostSortComments:
		return []OrderByClause{pinned, table.Post.CommentsCount.DESC(), table.Post.ActiveAt.DESC(), table.Post.ID.DESC()}
	default:
		return []OrderByClause{pinned, table.Post.ActiveAt.DESC(), table.Post.ID.DESC()}
	}
}

func (r *postRepository) List(
	ctx context.Context,
	filter PostFilter,
	limit, offset int64,
) (total int64, items []domain.PostListItem, err error) {
	defer func() { err = storageError(err, "post.List") }()
	condition := filter.condition()
	from := postFrom(filter)
	countStmt := SELECT(COUNT(STAR)).FROM(from).WHERE(condition)
	var count struct{ Count int64 }
	if err := countStmt.QueryContext(ctx, queryDB(ctx, r.db), &count); err != nil {
		return 0, nil, err
	}

	stmt := SELECT(
		table.Post.ID, table.Post.CategoryID, table.Post.Title,
		table.Post.AuthorID, table.Post.AuthorUsername, table.Post.Status,
		table.Post.ViewsCount, table.Post.CommentsCount, table.Post.CommentsLocked,
		table.Post.PinOrder, table.Post.CreatedAt, table.Post.UpdatedAt, table.Post.ActiveAt,
	).
		FROM(from).
		WHERE(condition).
		ORDER_BY(postOrderBy(filter.Sort)...).
		LIMIT(limit).
		OFFSET(offset)
	var records []model.Post
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &records); err != nil {
		return 0, nil, err
	}
	postIDs := make([]int64, len(records))
	for i, record := range records {
		postIDs[i] = record.ID
	}
	tagsByPostID, err := r.tagRepo.ListForPosts(ctx, postIDs)
	if err != nil {
		return 0, nil, err
	}
	dest := make([]domain.PostListItem, len(records))
	for i, record := range records {
		dest[i] = postListItemFromModel(record, tagsByPostID[record.ID])
	}
	return count.Count, dest, nil
}

func (r *postRepository) ExistsPublished(ctx context.Context, id int64) (exists bool, err error) {
	defer func() { err = storageError(err, "post.ExistsPublished") }()
	var result struct{ Exists bool }
	stmt := SELECT(EXISTS(SELECT(table.Post.ID).
		FROM(table.Post).
		WHERE(table.Post.ID.EQ(Int64(id)).AND(table.Post.Status.EQ(Int16(int16(domain.PostStatusPublished)))))).AS("Exists"))
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &result); err != nil {
		return false, err
	}
	return result.Exists, nil
}

func (r *postRepository) Find(
	ctx context.Context,
	id int64,
	incrementViews bool,
) (result *domain.Post, err error) {
	defer func() { err = storageError(err, "post.Find") }()
	var dest model.Post
	if incrementViews {
		stmt := table.Post.UPDATE(table.Post.ViewsCount).
			SET(table.Post.ViewsCount.ADD(Int32(1))).
			WHERE(table.Post.ID.EQ(Int64(id)).AND(table.Post.Status.EQ(Int16(int16(domain.PostStatusPublished))))).
			RETURNING(table.Post.AllColumns)
		err = stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest)
	} else {
		stmt := SELECT(table.Post.AllColumns).
			FROM(table.Post).
			WHERE(table.Post.ID.EQ(Int64(id)).AND(table.Post.Status.EQ(Int16(int16(domain.PostStatusPublished)))))
		err = stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest)
	}
	if err != nil {
		return nil, err
	}
	tags, err := r.tagRepo.ListForPost(ctx, id)
	if err != nil {
		return nil, err
	}
	post := postFromModel(dest, tags)
	return &post, nil
}

func (r *postRepository) Create(ctx context.Context, input CreatePostInput) (result *domain.Post, err error) {
	defer func() { err = storageError(err, "post.Create") }()
	if _, ok := forumcategory.FindByID(input.CategoryID); !ok {
		return nil, ErrInvalidCategory
	}
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}

	record := model.Post{
		CategoryID:     input.CategoryID,
		Title:          input.Title,
		AuthorID:       input.AuthorID,
		AuthorUsername: input.AuthorUsername,
		Content:        input.Content,
		Attr:           input.Attr,
	}
	insert := table.Post.INSERT(
		table.Post.CategoryID,
		table.Post.Title,
		table.Post.AuthorID,
		table.Post.AuthorUsername,
		table.Post.Content,
		table.Post.Attr,
	).
		MODEL(record).
		RETURNING(table.Post.AllColumns)
	if err := insert.QueryContext(ctx, tx, &record); err != nil {
		return nil, err
	}
	if err := replacePostTags(ctx, tx, record.ID, record.CategoryID, input.TagIDs); err != nil {
		return nil, err
	}
	tags, err := listPostTags(ctx, tx, record.ID)
	if err != nil {
		return nil, err
	}
	post := postFromModel(record, tags)
	return &post, nil
}

func replacePostTags(ctx context.Context, db qrm.DB, postID, categoryID int64, tagIDs []int64) error {
	deleteStmt := table.PostTag.DELETE().WHERE(table.PostTag.PostID.EQ(Int64(postID)))
	if _, err := deleteStmt.ExecContext(ctx, db); err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}
	validStmt := SELECT(table.Tag.AllColumns).
		FROM(table.Tag).
		WHERE(table.Tag.CategoryID.EQ(Int64(categoryID)).
			AND(table.Tag.IsActive.IS_TRUE()).
			AND(table.Tag.ID.IN(integerExpressions(tagIDs)...)))
	var valid []Tag
	if err := validStmt.QueryContext(ctx, db, &valid); err != nil {
		return err
	}
	if len(valid) != len(tagIDs) {
		return ErrInvalidTag
	}
	links := make([]model.PostTag, len(valid))
	for i, tag := range valid {
		links[i] = model.PostTag{PostID: postID, TagID: tag.ID}
	}
	insertStmt := table.PostTag.INSERT(table.PostTag.PostID, table.PostTag.TagID).MODELS(links)
	_, err := insertStmt.ExecContext(ctx, db)
	return err
}

func (r *postRepository) Update(
	ctx context.Context,
	id int64,
	input UpdatePostInput,
) (result *domain.Post, err error) {
	defer func() { err = storageError(err, "post.Update") }()
	if _, ok := forumcategory.FindByID(input.CategoryID); !ok {
		return nil, ErrInvalidCategory
	}
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	stmt := table.Post.UPDATE(table.Post.CategoryID, table.Post.Title, table.Post.Content, table.Post.UpdatedAt).
		SET(Int64(input.CategoryID), String(input.Title), String(input.Content), TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)).AND(table.Post.Status.EQ(Int16(int16(domain.PostStatusPublished))))).
		RETURNING(table.Post.AllColumns)
	var record model.Post
	if err := stmt.QueryContext(ctx, tx, &record); err != nil {
		return nil, err
	}
	if err := replacePostTags(ctx, tx, id, input.CategoryID, input.TagIDs); err != nil {
		return nil, err
	}
	tags, err := listPostTags(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	post := postFromModel(record, tags)
	return &post, nil
}

func (r *postRepository) SetStatus(ctx context.Context, id int64, status domain.PostStatus) (err error) {
	defer func() { err = storageError(err, "post.SetStatus") }()
	stmt := table.Post.UPDATE(table.Post.Status, table.Post.UpdatedAt).
		SET(Int16(int16(status)), TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)))
	return execPostUpdate(ctx, queryDB(ctx, r.db), stmt)
}

func (r *postRepository) SetCommentsLocked(ctx context.Context, id int64, locked bool) (err error) {
	defer func() { err = storageError(err, "post.SetCommentsLocked") }()
	stmt := table.Post.UPDATE(table.Post.CommentsLocked, table.Post.UpdatedAt).
		SET(Bool(locked), TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)))
	return execPostUpdate(ctx, queryDB(ctx, r.db), stmt)
}

func (r *postRepository) SetPinOrder(ctx context.Context, id int64, pinOrder *int32) (err error) {
	defer func() { err = storageError(err, "post.SetPinOrder") }()
	stmt := table.Post.UPDATE(table.Post.PinOrder, table.Post.UpdatedAt).
		SET(pinOrder, TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)))
	return execPostUpdate(ctx, queryDB(ctx, r.db), stmt)
}

func execPostUpdate(ctx context.Context, db qrm.DB, stmt UpdateStatement) error {
	result, err := stmt.ExecContext(ctx, db)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return qrm.ErrNoRows
	}
	return nil
}
