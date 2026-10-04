package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	"forum/internal/domain"

	. "github.com/go-jet/jet/v2/postgres"
)

const CommentReplyPageSize int64 = 20

type CommentFilter struct {
	Search, AuthorName string
	PostID             int64
	Status             *domain.CommentStatus
}

// Create and SetStatus require a context provided by WithinTransaction.
type CommentRepository interface {
	ListAdmin(ctx context.Context, filter CommentFilter, limit, offset int64) (int64, []domain.Comment, error)
	ListRoots(
		ctx context.Context,
		subjectType domain.CommentSubjectType,
		subjectKey string,
		limit, offset int64,
	) (int64, []domain.CommentThreadPreview, error)
	ListReplies(
		ctx context.Context,
		subjectType domain.CommentSubjectType,
		subjectKey string,
		rootID, limit, offset int64,
	) (int64, []domain.Comment, error)
	Find(ctx context.Context, subjectType domain.CommentSubjectType, id int64) (*domain.Comment, error)
	Create(ctx context.Context, comment domain.Comment) (*domain.Comment, error)
	Update(
		ctx context.Context,
		subjectType domain.CommentSubjectType,
		id int64,
		content string,
	) (*domain.Comment, error)
	SetStatus(ctx context.Context, subjectType domain.CommentSubjectType, id int64, status domain.CommentStatus) error
	DeleteAllByAuthor(ctx context.Context, authorID int64) error
}

type commentRepository struct{ db *sql.DB }

func NewCommentRepository(db *sql.DB) CommentRepository { return &commentRepository{db: db} }

func (r *commentRepository) ListRoots(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	subjectKey string,
	limit, offset int64,
) (total int64, items []domain.CommentThreadPreview, err error) {
	defer func() { err = storageError(err, "comment.ListRoots") }()
	condition := table.Comment.SubjectType.EQ(Int16(int16(subjectType))).
		AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
		AND(table.Comment.RootID.IS_NULL())
	countStmt := SELECT(COUNT(STAR)).FROM(table.Comment).WHERE(condition)
	var count struct{ Count int64 }
	if err := countStmt.QueryContext(ctx, queryDB(ctx, r.db), &count); err != nil {
		return 0, nil, err
	}
	stmt := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(condition).
		ORDER_BY(table.Comment.CreatedAt.ASC(), table.Comment.ID.ASC()).
		LIMIT(limit).
		OFFSET(offset)
	var dest []model.Comment
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return 0, nil, err
	}
	rootIDs := make([]int64, len(dest))
	for i, comment := range dest {
		rootIDs[i] = comment.ID
	}
	type replyCountRow struct {
		RootID *int64
		Count  int64
	}
	var replyCounts []replyCountRow
	if len(rootIDs) > 0 {
		err := SELECT(
			table.Comment.RootID.AS("replyCountRow.RootID"),
			COUNT(STAR).AS("replyCountRow.Count"),
		).
			FROM(table.Comment).
			WHERE(table.Comment.SubjectType.EQ(Int16(int16(subjectType))).
				AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
				AND(table.Comment.RootID.IN(integerExpressions(rootIDs)...))).
			GROUP_BY(table.Comment.RootID).
			QueryContext(ctx, queryDB(ctx, r.db), &replyCounts)
		if err != nil {
			return 0, nil, err
		}
	}
	countsByRoot := make(map[int64]int64, len(replyCounts))
	for _, row := range replyCounts {
		if row.RootID != nil {
			countsByRoot[*row.RootID] = row.Count
		}
	}
	// Fetch the first page for all roots in this page in one query.
	repliesByRoot := make(map[int64][]domain.Comment, len(rootIDs))
	if len(rootIDs) > 0 {
		ranked := SELECT(table.Comment.AllColumns,
			ROW_NUMBER().OVER(PARTITION_BY(table.Comment.RootID).
				ORDER_BY(table.Comment.CreatedAt.ASC(), table.Comment.ID.ASC())).AS("reply_rank"),
		).FROM(table.Comment).
			WHERE(table.Comment.SubjectType.EQ(Int16(int16(subjectType))).
				AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
				AND(table.Comment.RootID.IN(integerExpressions(rootIDs)...))).AsTable("ranked")
		var replies []model.Comment
		err := SELECT(ranked.AllColumns()).FROM(ranked).
			WHERE(IntegerColumn("reply_rank").From(ranked).LT_EQ(Int64(CommentReplyPageSize))).
			ORDER_BY(table.Comment.CreatedAt.From(ranked).ASC(), table.Comment.ID.From(ranked).ASC()).
			QueryContext(ctx, queryDB(ctx, r.db), &replies)
		if err != nil {
			return 0, nil, err
		}
		for _, reply := range replies {
			repliesByRoot[*reply.RootID] = append(repliesByRoot[*reply.RootID], commentFromModel(reply))
		}
	}
	threads := make([]domain.CommentThreadPreview, len(dest))
	for i, comment := range dest {
		threads[i] = domain.CommentThreadPreview{
			Root:       commentFromModel(comment),
			ReplyCount: countsByRoot[comment.ID],
			Replies:    repliesByRoot[comment.ID],
		}
	}
	return count.Count, threads, nil
}

func (r *commentRepository) ListReplies(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	subjectKey string,
	rootID, limit, offset int64,
) (total int64, items []domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.ListReplies") }()
	rootStmt := SELECT(table.Comment.ID).FROM(table.Comment).WHERE(
		table.Comment.ID.EQ(Int64(rootID)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType)))).
			AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
			AND(table.Comment.RootID.IS_NULL()),
	)
	var root struct{ ID int64 }
	if err := rootStmt.QueryContext(ctx, queryDB(ctx, r.db), &root); err != nil {
		return 0, nil, err
	}
	condition := table.Comment.SubjectType.EQ(Int16(int16(subjectType))).
		AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
		AND(table.Comment.RootID.EQ(Int64(rootID)))
	var count struct{ Count int64 }
	if err := SELECT(COUNT(STAR)).FROM(table.Comment).WHERE(condition).QueryContext(ctx, queryDB(ctx, r.db), &count); err != nil {
		return 0, nil, err
	}
	var dest []model.Comment
	if err = SELECT(table.Comment.AllColumns).FROM(table.Comment).WHERE(condition).
		ORDER_BY(table.Comment.CreatedAt.ASC(), table.Comment.ID.ASC()).
		LIMIT(limit).OFFSET(offset).QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return 0, nil, err
	}
	return count.Count, commentsFromModels(dest), nil
}

func (r *commentRepository) ListAdmin(
	ctx context.Context,
	filter CommentFilter,
	limit, offset int64,
) (total int64, items []domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.ListAdmin") }()
	condition := table.Comment.SubjectType.EQ(Int16(int16(domain.CommentSubjectPost)))
	if filter.PostID > 0 {
		condition = condition.AND(table.Comment.SubjectKey.EQ(String(domain.PostCommentSubjectKey(filter.PostID))))
	}
	if filter.Status != nil {
		condition = condition.AND(table.Comment.Status.EQ(Int16(int16(*filter.Status))))
	}
	if filter.Search != "" {
		condition = condition.AND(RawBool(`"comment"."content" ILIKE :contentPattern`, RawArgs{":contentPattern": "%" + filter.Search + "%"}))
	}
	if filter.AuthorName != "" {
		condition = condition.AND(RawBool(`"comment"."author_username" ILIKE :authorPattern`, RawArgs{":authorPattern": "%" + filter.AuthorName + "%"}))
	}
	var count struct{ Count int64 }
	if err := SELECT(COUNT(STAR)).FROM(table.Comment).WHERE(condition).QueryContext(ctx, queryDB(ctx, r.db), &count); err != nil {
		return 0, nil, err
	}
	var dest []model.Comment
	err = SELECT(table.Comment.AllColumns).FROM(table.Comment).WHERE(condition).
		ORDER_BY(table.Comment.CreatedAt.DESC(), table.Comment.ID.DESC()).
		LIMIT(limit).
		OFFSET(offset).
		QueryContext(ctx, queryDB(ctx, r.db), &dest)
	return count.Count, commentsFromModels(dest), err
}

func (r *commentRepository) Find(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	id int64,
) (result *domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.Find") }()
	stmt := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType)))))
	var dest model.Comment
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return nil, err
	}
	converted := commentFromModel(dest)
	return &converted, nil
}

func (r *commentRepository) Create(ctx context.Context, input domain.Comment) (result *domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.Create") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	var postID int64
	if input.SubjectType == domain.CommentSubjectPost {
		postID, err = domain.PostIDFromCommentSubjectKey(input.SubjectKey)
		if err != nil {
			return nil, err
		}
		lockPost := SELECT(table.Post.AllColumns).
			FROM(table.Post).
			WHERE(table.Post.ID.EQ(Int64(postID)).AND(table.Post.Status.EQ(Int16(StatusPublished)))).
			FOR(UPDATE())
		var post model.Post
		if err := lockPost.QueryContext(ctx, tx, &post); err != nil {
			return nil, err
		}
		if post.CommentsLocked {
			return nil, ErrCommentsLocked
		}
	}
	if input.RootID != nil {
		rootStmt := SELECT(table.Comment.AllColumns).
			FROM(table.Comment).
			WHERE(table.Comment.ID.EQ(Int64(*input.RootID)).
				AND(table.Comment.SubjectType.EQ(Int16(int16(input.SubjectType)))).
				AND(table.Comment.Status.EQ(Int16(StatusPublished))))
		var root model.Comment
		if err := rootStmt.QueryContext(ctx, tx, &root); err != nil {
			if errors.Is(storageError(err, "find root"), ErrNotFound) {
				return nil, ErrCommentRootNotFound
			}
			return nil, err
		}
		if root.SubjectKey != input.SubjectKey || root.RootID != nil {
			return nil, ErrInvalidCommentRoot
		}
	}
	record := model.Comment{
		SubjectType:    int16(input.SubjectType),
		SubjectKey:     input.SubjectKey,
		RootID:         input.RootID,
		Content:        input.Content,
		AuthorID:       input.AuthorID,
		AuthorUsername: input.AuthorUsername,
		Attr:           "{}",
	}
	insert := table.Comment.INSERT(
		table.Comment.SubjectType,
		table.Comment.SubjectKey,
		table.Comment.RootID,
		table.Comment.Content,
		table.Comment.AuthorID,
		table.Comment.AuthorUsername,
		table.Comment.Attr,
	).
		MODEL(record).
		RETURNING(table.Comment.AllColumns)
	if err := insert.QueryContext(ctx, tx, &record); err != nil {
		return nil, err
	}
	if input.SubjectType == domain.CommentSubjectPost {
		touchPost := table.Post.UPDATE(table.Post.CommentsCount, table.Post.ActiveAt).
			SET(table.Post.CommentsCount.ADD(Int32(1)), TimestampzT(time.Now())).
			WHERE(table.Post.ID.EQ(Int64(postID)))
		if _, err := touchPost.ExecContext(ctx, tx); err != nil {
			return nil, err
		}
	}
	converted := commentFromModel(record)
	return &converted, nil
}

func (r *commentRepository) Update(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	id int64,
	content string,
) (result *domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.Update") }()
	stmt := table.Comment.UPDATE(table.Comment.Content, table.Comment.UpdatedAt).
		SET(String(content), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType)))).
			AND(table.Comment.Status.EQ(Int16(StatusPublished)))).
		RETURNING(table.Comment.AllColumns)
	var dest model.Comment
	if err := stmt.QueryContext(ctx, queryDB(ctx, r.db), &dest); err != nil {
		return nil, err
	}
	converted := commentFromModel(dest)
	return &converted, nil
}

func (r *commentRepository) SetStatus(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	id int64,
	status domain.CommentStatus,
) (err error) {
	defer func() { err = storageError(err, "comment.SetStatus") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return err
	}
	lockComment := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType))))).
		FOR(UPDATE())
	var record model.Comment
	if err := lockComment.QueryContext(ctx, tx, &record); err != nil {
		return err
	}
	updateComment := table.Comment.UPDATE(table.Comment.Status, table.Comment.UpdatedAt).
		SET(Int16(int16(status)), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType)))))
	if _, err := updateComment.ExecContext(ctx, tx); err != nil {
		return err
	}
	if domain.CommentSubjectType(record.SubjectType) != domain.CommentSubjectPost ||
		domain.CommentStatus(record.Status) == status ||
		(record.Status != StatusPublished && status != domain.CommentStatusPublished) {
		return nil
	}
	postID, err := domain.PostIDFromCommentSubjectKey(record.SubjectKey)
	if err != nil {
		return err
	}
	if record.Status == StatusPublished {
		updatePost := table.Post.UPDATE(table.Post.CommentsCount).
			SET(IntExp(GREATEST(table.Post.CommentsCount.SUB(Int32(1)), Int32(0)))).
			WHERE(table.Post.ID.EQ(Int64(postID)))
		if _, err := updatePost.ExecContext(ctx, tx); err != nil {
			return err
		}
	} else if status == domain.CommentStatusPublished {
		updatePost := table.Post.UPDATE(table.Post.CommentsCount, table.Post.ActiveAt).
			SET(table.Post.CommentsCount.ADD(Int32(1)), TimestampzT(time.Now())).
			WHERE(table.Post.ID.EQ(Int64(postID)))
		if _, err := updatePost.ExecContext(ctx, tx); err != nil {
			return err
		}
	}
	return nil
}

func (r *commentRepository) DeleteAllByAuthor(ctx context.Context, authorID int64) (err error) {
	defer func() { err = storageError(err, "comment.DeleteAllByAuthor") }()
	_, err = queryDB(ctx, r.db).ExecContext(ctx, `
		WITH published_comments AS (
			UPDATE comment
			SET status = $2, updated_at = CURRENT_TIMESTAMP
			WHERE author_id = $1 AND status = $3
			RETURNING subject_type, subject_key
		), hidden_comments AS (
			UPDATE comment
			SET status = $2, updated_at = CURRENT_TIMESTAMP
			WHERE author_id = $1 AND status = $4
		), deleted_post_comments AS (
			SELECT subject_key, COUNT(*)::integer AS count
			FROM published_comments
			WHERE subject_type = $5
			GROUP BY subject_key
		)
		UPDATE post
		SET comments_count = GREATEST(post.comments_count - deleted_post_comments.count, 0)
		FROM deleted_post_comments
		WHERE post.id::text = deleted_post_comments.subject_key
	`, authorID, StatusDeleted, StatusPublished, StatusHidden, domain.CommentSubjectPost)
	return err
}

func commentFromModel(value model.Comment) domain.Comment {
	return domain.Comment{
		ID:             value.ID,
		SubjectType:    domain.CommentSubjectType(value.SubjectType),
		SubjectKey:     value.SubjectKey,
		RootID:         value.RootID,
		Content:        value.Content,
		AuthorID:       value.AuthorID,
		AuthorUsername: value.AuthorUsername,
		Status:         domain.CommentStatus(value.Status),
		CreatedAt:      value.CreatedAt,
		UpdatedAt:      value.UpdatedAt,
	}
}

func commentsFromModels(values []model.Comment) []domain.Comment {
	comments := make([]domain.Comment, len(values))
	for i, value := range values {
		comments[i] = commentFromModel(value)
	}
	return comments
}
