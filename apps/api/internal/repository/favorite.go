package repository

import (
	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	"database/sql"

	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
)

type FavoriteRepository interface {
	Has(postID, userID int64) (bool, error)
	ListPostIDs(userID int64, postIDs []int64) (map[int64]bool, error)
	Set(postID, userID int64, favorite bool) error
}

type favoriteRepository struct{ db *sql.DB }

func NewFavoriteRepository(db *sql.DB) FavoriteRepository { return &favoriteRepository{db: db} }

func (r *favoriteRepository) Has(postID, userID int64) (bool, error) {
	var result struct{ Exists bool }
	stmt := SELECT(EXISTS(SELECT(table.PostFavorite.PostID).
		FROM(table.PostFavorite).
		WHERE(table.PostFavorite.PostID.EQ(Int64(postID)).
			AND(table.PostFavorite.UserID.EQ(Int64(userID))))).AS("Exists"))
	if err := stmt.Query(r.db, &result); err != nil {
		return false, err
	}
	return result.Exists, nil
}

func (r *favoriteRepository) ListPostIDs(userID int64, postIDs []int64) (map[int64]bool, error) {
	result := make(map[int64]bool, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}
	stmt := SELECT(table.PostFavorite.PostID).
		FROM(table.PostFavorite).
		WHERE(table.PostFavorite.UserID.EQ(Int64(userID)).
			AND(table.PostFavorite.PostID.IN(integerExpressions(postIDs)...)))
	var records []model.PostFavorite
	if err := stmt.Query(r.db, &records); err != nil {
		return nil, err
	}
	for _, record := range records {
		result[record.PostID] = true
	}
	return result, nil
}

func (r *favoriteRepository) Set(postID, userID int64, favorite bool) error {
	if !favorite {
		stmt := table.PostFavorite.DELETE().
			WHERE(table.PostFavorite.PostID.EQ(Int64(postID)).AND(table.PostFavorite.UserID.EQ(Int64(userID))))
		_, err := stmt.Exec(r.db)
		return err
	}
	var exists struct{ Exists bool }
	findPost := SELECT(EXISTS(SELECT(table.Post.ID).
		FROM(table.Post).
		WHERE(table.Post.ID.EQ(Int64(postID)).AND(table.Post.Status.EQ(Int16(StatusPublished))))).AS("Exists"))
	if err := findPost.Query(r.db, &exists); err != nil {
		return err
	}
	if !exists.Exists {
		return qrm.ErrNoRows
	}
	record := model.PostFavorite{PostID: postID, UserID: userID}
	stmt := table.PostFavorite.INSERT(table.PostFavorite.PostID, table.PostFavorite.UserID).
		MODEL(record).
		ON_CONFLICT(table.PostFavorite.PostID, table.PostFavorite.UserID).
		DO_NOTHING()
	_, err := stmt.Exec(r.db)
	return err
}
