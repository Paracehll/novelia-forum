package repository

import (
	"context"
	"database/sql"
	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"

	. "github.com/go-jet/jet/v2/postgres"
)

type FavoriteRepository interface {
	Has(ctx context.Context, postID, userID int64) (bool, error)
	ListPostIDs(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error)
	Set(ctx context.Context, postID, userID int64, favorite bool) error
}

type favoriteRepository struct{ db *sql.DB }

func NewFavoriteRepository(db *sql.DB) FavoriteRepository { return &favoriteRepository{db: db} }

func (r *favoriteRepository) Has(ctx context.Context, postID, userID int64) (exists bool, err error) {
	defer func() { err = storageError(err, "favorite.Has") }()
	var result struct{ Exists bool }
	stmt := SELECT(EXISTS(SELECT(table.PostFavorite.PostID).
		FROM(table.PostFavorite).
		WHERE(table.PostFavorite.PostID.EQ(Int64(postID)).
			AND(table.PostFavorite.UserID.EQ(Int64(userID))))).AS("Exists"))
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &result); err != nil {
		return false, err
	}
	return result.Exists, nil
}

func (r *favoriteRepository) ListPostIDs(
	ctx context.Context,
	userID int64,
	postIDs []int64,
) (favorites map[int64]bool, err error) {
	defer func() { err = storageError(err, "favorite.ListPostIDs") }()
	result := make(map[int64]bool, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}
	stmt := SELECT(table.PostFavorite.PostID).
		FROM(table.PostFavorite).
		WHERE(table.PostFavorite.UserID.EQ(Int64(userID)).
			AND(table.PostFavorite.PostID.IN(integerExpressions(postIDs)...)))
	var records []model.PostFavorite
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &records); err != nil {
		return nil, err
	}
	for _, record := range records {
		result[record.PostID] = true
	}
	return result, nil
}

func (r *favoriteRepository) Set(ctx context.Context, postID, userID int64, favorite bool) (err error) {
	defer func() { err = storageError(err, "favorite.Set") }()
	if !favorite {
		stmt := table.PostFavorite.DELETE().
			WHERE(table.PostFavorite.PostID.EQ(Int64(postID)).AND(table.PostFavorite.UserID.EQ(Int64(userID))))
		_, err := stmt.ExecContext(ctx, executor(ctx, r.db))
		return err
	}
	record := model.PostFavorite{PostID: postID, UserID: userID}
	stmt := table.PostFavorite.INSERT(table.PostFavorite.PostID, table.PostFavorite.UserID).
		MODEL(record).
		ON_CONFLICT(table.PostFavorite.PostID, table.PostFavorite.UserID).
		DO_NOTHING()
	_, err = stmt.ExecContext(ctx, executor(ctx, r.db))
	return err
}
