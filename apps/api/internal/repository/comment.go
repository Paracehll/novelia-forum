package repository

import (
	"auth/.gen/main/public/model"
	"auth/.gen/main/public/table"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	. "github.com/go-jet/jet/v2/postgres"
)

type Comment = model.Comment

type CommentThread struct {
	Comment
	ReplyCount int64
	Replies    []Comment
}

const CommentReplyPageSize int64 = 20

const (
	CommentSubjectPost  int16 = 0
	CommentSubjectNovel int16 = 1
)

type CreateCommentInput struct {
	SubjectType    int16
	SubjectKey     string
	RootID         *int64
	Content        string
	AuthorID       int64
	AuthorUsername string
	Attr           string
}

type CommentFilter struct {
	Search, AuthorName string
	PostID             int64
	Status             int16
}

const CommentStatusAll int16 = -1

type CommentRepository interface {
	ListAdmin(filter CommentFilter, limit, offset int64) (int64, []Comment, error)
	ListRoots(subjectType int16, subjectKey string, limit, offset int64) (int64, []CommentThread, error)
	ListReplies(subjectType int16, subjectKey string, rootID, limit, offset int64) (int64, []Comment, error)
	Find(subjectType int16, id int64) (*Comment, error)
	Create(input CreateCommentInput) (*Comment, error)
	Update(subjectType int16, id int64, content string) (*Comment, error)
	SetStatus(subjectType int16, id int64, status int16) error
	DeleteAllByAuthor(authorID int64) error
}

type commentRepository struct{ db *sql.DB }

func NewCommentRepository(db *sql.DB) CommentRepository { return &commentRepository{db: db} }

func (r *commentRepository) ListRoots(subjectType int16, subjectKey string, limit, offset int64) (total int64, items []CommentThread, err error) {
	defer func() { err = storageError(err, "comment.ListRoots") }()
	condition := table.Comment.SubjectType.EQ(Int16(subjectType)).
		AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
		AND(table.Comment.RootID.IS_NULL())
	countStmt := SELECT(COUNT(STAR)).FROM(table.Comment).WHERE(condition)
	var count struct{ Count int64 }
	if err := countStmt.Query(r.db, &count); err != nil {
		return 0, nil, err
	}
	stmt := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(condition).
		ORDER_BY(table.Comment.CreatedAt.ASC(), table.Comment.ID.ASC()).
		LIMIT(limit).
		OFFSET(offset)
	var dest []Comment
	if err := stmt.Query(r.db, &dest); err != nil {
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
			WHERE(table.Comment.SubjectType.EQ(Int16(subjectType)).
				AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
				AND(table.Comment.RootID.IN(integerExpressions(rootIDs)...))).
			GROUP_BY(table.Comment.RootID).
			Query(r.db, &replyCounts)
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
	repliesByRoot := make(map[int64][]Comment, len(rootIDs))
	if len(rootIDs) > 0 {
		ranked := SELECT(table.Comment.AllColumns,
			ROW_NUMBER().OVER(PARTITION_BY(table.Comment.RootID).
				ORDER_BY(table.Comment.CreatedAt.ASC(), table.Comment.ID.ASC())).AS("reply_rank"),
		).FROM(table.Comment).
			WHERE(table.Comment.SubjectType.EQ(Int16(subjectType)).
				AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
				AND(table.Comment.RootID.IN(integerExpressions(rootIDs)...))).AsTable("ranked")
		var replies []Comment
		err := SELECT(ranked.AllColumns()).FROM(ranked).
			WHERE(IntegerColumn("reply_rank").From(ranked).LT_EQ(Int64(CommentReplyPageSize))).
			ORDER_BY(table.Comment.CreatedAt.From(ranked).ASC(), table.Comment.ID.From(ranked).ASC()).
			Query(r.db, &replies)
		if err != nil {
			return 0, nil, err
		}
		for _, reply := range replies {
			repliesByRoot[*reply.RootID] = append(repliesByRoot[*reply.RootID], reply)
		}
	}
	threads := make([]CommentThread, len(dest))
	for i, comment := range dest {
		threads[i] = CommentThread{Comment: comment, ReplyCount: countsByRoot[comment.ID], Replies: repliesByRoot[comment.ID]}
	}
	return count.Count, threads, nil
}

func (r *commentRepository) ListReplies(subjectType int16, subjectKey string, rootID, limit, offset int64) (total int64, items []Comment, err error) {
	defer func() { err = storageError(err, "comment.ListReplies") }()
	rootStmt := SELECT(table.Comment.ID).FROM(table.Comment).WHERE(
		table.Comment.ID.EQ(Int64(rootID)).
			AND(table.Comment.SubjectType.EQ(Int16(subjectType))).
			AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
			AND(table.Comment.RootID.IS_NULL()),
	)
	var root struct{ ID int64 }
	if err := rootStmt.Query(r.db, &root); err != nil {
		return 0, nil, err
	}
	condition := table.Comment.SubjectType.EQ(Int16(subjectType)).
		AND(table.Comment.SubjectKey.EQ(String(subjectKey))).
		AND(table.Comment.RootID.EQ(Int64(rootID)))
	var count struct{ Count int64 }
	if err := SELECT(COUNT(STAR)).FROM(table.Comment).WHERE(condition).Query(r.db, &count); err != nil {
		return 0, nil, err
	}
	var dest []Comment
	if err = SELECT(table.Comment.AllColumns).FROM(table.Comment).WHERE(condition).
		ORDER_BY(table.Comment.CreatedAt.ASC(), table.Comment.ID.ASC()).
		LIMIT(limit).OFFSET(offset).Query(r.db, &dest); err != nil {
		return 0, nil, err
	}
	return count.Count, dest, nil
}

func (r *commentRepository) ListAdmin(filter CommentFilter, limit, offset int64) (total int64, items []Comment, err error) {
	defer func() { err = storageError(err, "comment.ListAdmin") }()
	condition := table.Comment.SubjectType.EQ(Int16(CommentSubjectPost))
	if filter.PostID > 0 {
		condition = condition.AND(table.Comment.SubjectKey.EQ(String(PostSubjectKey(filter.PostID))))
	}
	if filter.Status != CommentStatusAll {
		condition = condition.AND(table.Comment.Status.EQ(Int16(filter.Status)))
	}
	if filter.Search != "" {
		condition = condition.AND(RawBool(`"comment"."content" ILIKE :contentPattern`, RawArgs{":contentPattern": "%" + filter.Search + "%"}))
	}
	if filter.AuthorName != "" {
		condition = condition.AND(RawBool(`"comment"."author_username" ILIKE :authorPattern`, RawArgs{":authorPattern": "%" + filter.AuthorName + "%"}))
	}
	var count struct{ Count int64 }
	if err := SELECT(COUNT(STAR)).FROM(table.Comment).WHERE(condition).Query(r.db, &count); err != nil {
		return 0, nil, err
	}
	var dest []Comment
	err = SELECT(table.Comment.AllColumns).FROM(table.Comment).WHERE(condition).
		ORDER_BY(table.Comment.CreatedAt.DESC(), table.Comment.ID.DESC()).LIMIT(limit).OFFSET(offset).Query(r.db, &dest)
	return count.Count, dest, err
}

func (r *commentRepository) Find(subjectType int16, id int64) (result *Comment, err error) {
	defer func() { err = storageError(err, "comment.Find") }()
	stmt := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(subjectType))))
	var dest Comment
	if err := stmt.Query(r.db, &dest); err != nil {
		return nil, err
	}
	return &dest, nil
}

func (r *commentRepository) Create(input CreateCommentInput) (result *Comment, err error) {
	defer func() { err = storageError(err, "comment.Create") }()
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var postID int64
	if input.SubjectType == CommentSubjectPost {
		postID, err = PostIDFromSubjectKey(input.SubjectKey)
		if err != nil {
			return nil, err
		}
		lockPost := SELECT(table.Post.AllColumns).
			FROM(table.Post).
			WHERE(table.Post.ID.EQ(Int64(postID)).AND(table.Post.Status.EQ(Int16(StatusPublished)))).
			FOR(UPDATE())
		var post Post
		if err := lockPost.Query(tx, &post); err != nil {
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
				AND(table.Comment.SubjectType.EQ(Int16(input.SubjectType))).
				AND(table.Comment.Status.EQ(Int16(StatusPublished))))
		var root Comment
		if err := rootStmt.Query(tx, &root); err != nil {
			if errors.Is(storageError(err, "find root"), ErrNotFound) {
				return nil, ErrCommentRootNotFound
			}
			return nil, err
		}
		if root.SubjectKey != input.SubjectKey || root.RootID != nil {
			return nil, ErrInvalidCommentRoot
		}
	}
	record := Comment{
		SubjectType:    input.SubjectType,
		SubjectKey:     input.SubjectKey,
		RootID:         input.RootID,
		Content:        input.Content,
		AuthorID:       input.AuthorID,
		AuthorUsername: input.AuthorUsername,
		Attr:           input.Attr,
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
	if err := insert.Query(tx, &record); err != nil {
		return nil, err
	}
	if input.SubjectType == CommentSubjectPost {
		touchPost := table.Post.UPDATE(table.Post.CommentsCount, table.Post.ActiveAt).
			SET(table.Post.CommentsCount.ADD(Int32(1)), TimestampzT(time.Now())).
			WHERE(table.Post.ID.EQ(Int64(postID)))
		if _, err := touchPost.Exec(tx); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *commentRepository) Update(subjectType int16, id int64, content string) (result *Comment, err error) {
	defer func() { err = storageError(err, "comment.Update") }()
	stmt := table.Comment.UPDATE(table.Comment.Content, table.Comment.UpdatedAt).
		SET(String(content), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(subjectType))).
			AND(table.Comment.Status.EQ(Int16(StatusPublished)))).
		RETURNING(table.Comment.AllColumns)
	var dest Comment
	if err := stmt.Query(r.db, &dest); err != nil {
		return nil, err
	}
	return &dest, nil
}

func (r *commentRepository) SetStatus(subjectType int16, id int64, status int16) (err error) {
	defer func() { err = storageError(err, "comment.SetStatus") }()
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	lockComment := SELECT(table.Comment.AllColumns).
		FROM(table.Comment).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(subjectType)))).
		FOR(UPDATE())
	var record Comment
	if err := lockComment.Query(tx, &record); err != nil {
		return err
	}
	updateComment := table.Comment.UPDATE(table.Comment.Status, table.Comment.UpdatedAt).
		SET(Int16(status), TimestampzT(time.Now())).
		WHERE(table.Comment.ID.EQ(Int64(id)).
			AND(table.Comment.SubjectType.EQ(Int16(subjectType))))
	if _, err := updateComment.Exec(tx); err != nil {
		return err
	}
	if record.SubjectType != CommentSubjectPost ||
		record.Status == status ||
		(record.Status != StatusPublished && status != StatusPublished) {
		return tx.Commit()
	}
	postID, err := PostIDFromSubjectKey(record.SubjectKey)
	if err != nil {
		return err
	}
	if record.Status == StatusPublished {
		updatePost := table.Post.UPDATE(table.Post.CommentsCount).
			SET(IntExp(GREATEST(table.Post.CommentsCount.SUB(Int32(1)), Int32(0)))).
			WHERE(table.Post.ID.EQ(Int64(postID)))
		if _, err := updatePost.Exec(tx); err != nil {
			return err
		}
	} else if status == StatusPublished {
		updatePost := table.Post.UPDATE(table.Post.CommentsCount, table.Post.ActiveAt).
			SET(table.Post.CommentsCount.ADD(Int32(1)), TimestampzT(time.Now())).
			WHERE(table.Post.ID.EQ(Int64(postID)))
		if _, err := updatePost.Exec(tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *commentRepository) DeleteAllByAuthor(authorID int64) (err error) {
	defer func() { err = storageError(err, "comment.DeleteAllByAuthor") }()
	_, err = r.db.Exec(`
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
	`, authorID, StatusDeleted, StatusPublished, StatusHidden, CommentSubjectPost)
	return err
}

func PostSubjectKey(postID int64) string {
	return strconv.FormatInt(postID, 10)
}

func PostIDFromSubjectKey(subjectKey string) (int64, error) {
	postID, err := strconv.ParseInt(subjectKey, 10, 64)
	if err != nil || postID <= 0 {
		return 0, fmt.Errorf("invalid post subject key %q", subjectKey)
	}
	return postID, nil
}
