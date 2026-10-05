package repository

import (
	"context"
	"database/sql"
	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	"forum/internal/domain"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
)

const PostStatusAll = -1

type PostFilter struct {
	CategoryID               int64
	Search                   string
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
	Attr           string
}

type UpdatePostInput struct {
	CategoryID     int64
	Title, Content string
}

// Lock and write operations spanning multiple statements require a transaction context.
type PostRepository interface {
	List(ctx context.Context, filter PostFilter, limit, offset int64) (int64, []domain.PostListItem, error)
	Find(ctx context.Context, id int64, incrementViews bool) (*domain.Post, error)
	ExistsPublished(ctx context.Context, id int64) (bool, error)
	Lock(ctx context.Context, id int64) (*domain.Post, error)
	AdjustCommentsCount(ctx context.Context, id int64, delta int32, activeAt *time.Time) error
	ReplaceTags(ctx context.Context, postID int64, tagIDs []int64) error
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
func postFromModel(record model.Post, tags []domain.Tag) domain.Post {
	return domain.Post{
		ID: record.ID, CategoryID: record.CategoryID, Title: record.Title,
		AuthorID: record.AuthorID, AuthorUsername: record.AuthorUsername,
		Content: record.Content, Status: domain.PostStatus(record.Status),
		ViewsCount: record.ViewsCount, CommentsCount: record.CommentsCount,
		CommentsLocked: record.CommentsLocked, PinOrder: record.PinOrder,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ActiveAt: record.ActiveAt,
		Tags: domain.PostTags(tags),
	}
}

func postListItemFromModel(record model.Post, tags []domain.Tag) domain.PostListItem {
	return domain.PostListItem{
		ID: record.ID, CategoryID: record.CategoryID, Title: record.Title,
		AuthorID: record.AuthorID, AuthorUsername: record.AuthorUsername,
		Status:     domain.PostStatus(record.Status),
		ViewsCount: record.ViewsCount, CommentsCount: record.CommentsCount,
		CommentsLocked: record.CommentsLocked, PinOrder: record.PinOrder,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ActiveAt: record.ActiveAt,
		Tags: domain.PostTags(tags),
	}
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
	if filter.CategoryID > 0 {
		expressions = append(expressions, table.Post.CategoryID.EQ(Int64(filter.CategoryID)))
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
	case domain.PostSortNewest:
		return []OrderByClause{pinned, table.Post.CreatedAt.DESC(), table.Post.ID.DESC()}
	case domain.PostSortViews:
		return []OrderByClause{pinned, table.Post.ViewsCount.DESC(), table.Post.ActiveAt.DESC(), table.Post.ID.DESC()}
	case domain.PostSortComments:
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
	if err := countStmt.QueryContext(ctx, executor(ctx, r.db), &count); err != nil {
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
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &records); err != nil {
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
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &result); err != nil {
		return false, err
	}
	return result.Exists, nil
}

// Lock returns the stored state without imposing visibility or ownership rules.
func (r *postRepository) Lock(ctx context.Context, id int64) (result *domain.Post, err error) {
	defer func() { err = storageError(err, "post.Lock") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	var record model.Post
	if err := SELECT(table.Post.AllColumns).FROM(table.Post).
		WHERE(table.Post.ID.EQ(Int64(id))).FOR(UPDATE()).
		QueryContext(ctx, tx, &record); err != nil {
		return nil, err
	}
	post := postFromModel(record, nil)
	return &post, nil
}

func (r *postRepository) AdjustCommentsCount(ctx context.Context, id int64, delta int32, activeAt *time.Time) (err error) {
	defer func() { err = storageError(err, "post.AdjustCommentsCount") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return err
	}
	count := IntExp(GREATEST(table.Post.CommentsCount.ADD(Int32(delta)), Int32(0)))
	stmt := table.Post.UPDATE(table.Post.CommentsCount).SET(count).
		WHERE(table.Post.ID.EQ(Int64(id)))
	if activeAt != nil {
		stmt = table.Post.UPDATE(table.Post.CommentsCount, table.Post.ActiveAt).
			SET(count, TimestampzT(*activeAt)).WHERE(table.Post.ID.EQ(Int64(id)))
	}
	_, err = stmt.ExecContext(ctx, tx)
	return err
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
		err = stmt.QueryContext(ctx, executor(ctx, r.db), &dest)
	} else {
		stmt := SELECT(table.Post.AllColumns).
			FROM(table.Post).
			WHERE(table.Post.ID.EQ(Int64(id)).AND(table.Post.Status.EQ(Int16(int16(domain.PostStatusPublished)))))
		err = stmt.QueryContext(ctx, executor(ctx, r.db), &dest)
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
	post := postFromModel(record, nil)
	return &post, nil
}

func (r *postRepository) ReplaceTags(ctx context.Context, postID int64, tagIDs []int64) (err error) {
	defer func() { err = storageError(err, "post.ReplaceTags") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return err
	}
	deleteStmt := table.PostTag.DELETE().WHERE(table.PostTag.PostID.EQ(Int64(postID)))
	if _, err := deleteStmt.ExecContext(ctx, tx); err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}
	links := make([]model.PostTag, len(tagIDs))
	for i, tagID := range tagIDs {
		links[i] = model.PostTag{PostID: postID, TagID: tagID}
	}
	insertStmt := table.PostTag.INSERT(table.PostTag.PostID, table.PostTag.TagID).MODELS(links)
	_, err = insertStmt.ExecContext(ctx, tx)
	return err
}

func (r *postRepository) Update(
	ctx context.Context,
	id int64,
	input UpdatePostInput,
) (result *domain.Post, err error) {
	defer func() { err = storageError(err, "post.Update") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	stmt := table.Post.UPDATE(table.Post.CategoryID, table.Post.Title, table.Post.Content, table.Post.UpdatedAt).
		SET(Int64(input.CategoryID), String(input.Title), String(input.Content), TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id))).
		RETURNING(table.Post.AllColumns)
	var record model.Post
	if err := stmt.QueryContext(ctx, tx, &record); err != nil {
		return nil, err
	}
	post := postFromModel(record, nil)
	return &post, nil
}

func (r *postRepository) SetStatus(ctx context.Context, id int64, status domain.PostStatus) (err error) {
	defer func() { err = storageError(err, "post.SetStatus") }()
	stmt := table.Post.UPDATE(table.Post.Status, table.Post.UpdatedAt).
		SET(Int16(int16(status)), TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)))
	return requireAffectedRow(stmt.ExecContext(ctx, executor(ctx, r.db)))
}

func (r *postRepository) SetCommentsLocked(ctx context.Context, id int64, locked bool) (err error) {
	defer func() { err = storageError(err, "post.SetCommentsLocked") }()
	stmt := table.Post.UPDATE(table.Post.CommentsLocked, table.Post.UpdatedAt).
		SET(Bool(locked), TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)))
	return requireAffectedRow(stmt.ExecContext(ctx, executor(ctx, r.db)))
}

func (r *postRepository) SetPinOrder(ctx context.Context, id int64, pinOrder *int32) (err error) {
	defer func() { err = storageError(err, "post.SetPinOrder") }()
	stmt := table.Post.UPDATE(table.Post.PinOrder, table.Post.UpdatedAt).
		SET(pinOrder, TimestampzT(time.Now())).
		WHERE(table.Post.ID.EQ(Int64(id)))
	return requireAffectedRow(stmt.ExecContext(ctx, executor(ctx, r.db)))
}
