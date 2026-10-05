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

const CommentReplyPageSize int64 = 20

type CommentFilter struct {
	Search, AuthorName string
	PostID             int64
	Status             *domain.CommentStatus
}

// Lock and all write operations require a context provided by WithinTransaction.
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
	Lock(ctx context.Context, subjectType domain.CommentSubjectType, id int64) (*domain.Comment, error)
	Create(ctx context.Context, comment domain.Comment) (*domain.Comment, error)
	Update(
		ctx context.Context,
		subjectType domain.CommentSubjectType,
		id int64,
		content string,
	) (*domain.Comment, error)
	SetStatus(ctx context.Context, subjectType domain.CommentSubjectType, id int64, status domain.CommentStatus) error
	SetStatusByAuthor(ctx context.Context, authorID int64, from []domain.CommentStatus, to domain.CommentStatus) ([]domain.Comment, error)
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

func (r *commentRepository) Lock(
	ctx context.Context,
	subjectType domain.CommentSubjectType,
	id int64,
) (result *domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.Lock") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	stmt := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType))))).
		FOR(UPDATE())
	var record model.Comment
	if err := stmt.QueryContext(ctx, tx, &record); err != nil {
		return nil, err
	}
	comment := commentFromModel(record)
	return &comment, nil
}

func (r *commentRepository) Create(ctx context.Context, input domain.Comment) (result *domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.Create") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	record := model.Comment{
		SubjectType:    int16(input.SubjectType),
		SubjectKey:     input.SubjectKey,
		RootID:         input.RootID,
		Content:        input.Content,
		AuthorID:       input.AuthorID,
		AuthorUsername: input.AuthorUsername,
		Status:         int16(input.Status),
		Attr:           "{}",
	}
	insert := table.Comment.INSERT(
		table.Comment.SubjectType,
		table.Comment.SubjectKey,
		table.Comment.RootID,
		table.Comment.Content,
		table.Comment.AuthorID,
		table.Comment.AuthorUsername,
		table.Comment.Status,
		table.Comment.Attr,
	).
		MODEL(record).
		RETURNING(table.Comment.AllColumns)
	if err := insert.QueryContext(ctx, tx, &record); err != nil {
		return nil, err
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
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	stmt := table.Comment.UPDATE(table.Comment.Content, table.Comment.UpdatedAt).
		SET(String(content), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType))))).
		RETURNING(table.Comment.AllColumns)
	var dest model.Comment
	if err := stmt.QueryContext(ctx, tx, &dest); err != nil {
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
	stmt := table.Comment.UPDATE(table.Comment.Status, table.Comment.UpdatedAt).
		SET(Int16(int16(status)), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(int16(subjectType)))))
	result, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStatusByAuthor returns the locked rows as they were before the update.
// Callers decide which source states to change and handle any cross-table effects.
func (r *commentRepository) SetStatusByAuthor(
	ctx context.Context,
	authorID int64,
	from []domain.CommentStatus,
	to domain.CommentStatus,
) (comments []domain.Comment, err error) {
	defer func() { err = storageError(err, "comment.SetStatusByAuthor") }()
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return nil, err
	}
	if len(from) == 0 {
		return []domain.Comment{}, nil
	}
	states := make([]Expression, len(from))
	for i, status := range from {
		states[i] = Int16(int16(status))
	}
	stmt := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(table.Comment.AuthorID.EQ(Int64(authorID)).
			AND(table.Comment.Status.IN(states...))).
		ORDER_BY(table.Comment.ID.ASC()).
		FOR(UPDATE())
	var records []model.Comment
	if err := stmt.QueryContext(ctx, tx, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []domain.Comment{}, nil
	}
	ids := make([]int64, len(records))
	for i, record := range records {
		ids[i] = record.ID
	}
	update := table.Comment.UPDATE(table.Comment.Status, table.Comment.UpdatedAt).
		SET(Int16(int16(to)), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.IN(integerExpressions(ids)...))
	if _, err := update.ExecContext(ctx, tx); err != nil {
		return nil, err
	}
	return commentsFromModels(records), nil
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
