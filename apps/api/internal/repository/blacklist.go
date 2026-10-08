package repository

import (
	"context"
	"database/sql"

	"forum/.gen/main/public/model"
	"forum/.gen/main/public/table"
	"forum/internal/domain"

	. "github.com/go-jet/jet/v2/postgres"
)

type BlacklistRepository interface {
	LockUser(context.Context, int64) error
	List(context.Context, int64) ([]domain.BlacklistEntry, error)
	Add(context.Context, domain.BlacklistEntry) error
	Remove(context.Context, int64, int64) error
}

type blacklistRepository struct{ db *sql.DB }

func NewBlacklistRepository(db *sql.DB) BlacklistRepository {
	return &blacklistRepository{db: db}
}

// LockUser serializes changes for one owner across service instances. The lock
// lasts until the usecase transaction ends, including when the list is empty.
func (r *blacklistRepository) LockUser(ctx context.Context, userID int64) error {
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended('user_blacklist:' || $1::text, 0))`,
		userID,
	)
	return storageError(err, "blacklist.LockUser")
}

func (r *blacklistRepository) List(
	ctx context.Context,
	userID int64,
) ([]domain.BlacklistEntry, error) {
	stmt := SELECT(table.UserBlacklist.AllColumns).
		FROM(table.UserBlacklist).
		WHERE(table.UserBlacklist.UserID.EQ(Int64(userID))).
		ORDER_BY(
			table.UserBlacklist.CreatedAt.DESC(),
			table.UserBlacklist.BlockedUserID.DESC(),
		)
	var records []model.UserBlacklist
	if err := stmt.QueryContext(ctx, executor(ctx, r.db), &records); err != nil {
		return nil, storageError(err, "blacklist.List")
	}
	items := make([]domain.BlacklistEntry, len(records))
	for i, record := range records {
		items[i] = domain.BlacklistEntry{
			UserID:        record.UserID,
			BlockedUserID: record.BlockedUserID,
			CreatedAt:     record.CreatedAt,
		}
	}
	return items, nil
}

func (r *blacklistRepository) Add(ctx context.Context, entry domain.BlacklistEntry) error {
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return err
	}
	record := model.UserBlacklist{
		UserID:        entry.UserID,
		BlockedUserID: entry.BlockedUserID,
	}
	stmt := table.UserBlacklist.
		INSERT(table.UserBlacklist.UserID, table.UserBlacklist.BlockedUserID).
		MODEL(record)
	_, err = stmt.ExecContext(ctx, tx)
	return storageError(err, "blacklist.Add")
}

func (r *blacklistRepository) Remove(ctx context.Context, userID, blockedUserID int64) error {
	tx, err := requireTransaction(ctx, r.db)
	if err != nil {
		return err
	}
	stmt := table.UserBlacklist.DELETE().
		WHERE(table.UserBlacklist.UserID.EQ(Int64(userID)).
			AND(table.UserBlacklist.BlockedUserID.EQ(Int64(blockedUserID))))
	_, err = stmt.ExecContext(ctx, tx)
	return storageError(err, "blacklist.Remove")
}
