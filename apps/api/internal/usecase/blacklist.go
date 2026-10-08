package usecase

import (
	"context"
	"forum/internal/domain"
	"forum/internal/repository"
)

const (
	BlacklistLimit            = 1000
	CodeBlacklistActorInvalid = "blacklist.actor_invalid"
	CodeBlacklistUserInvalid  = "blacklist.user_invalid"
	CodeBlacklistSelf         = "blacklist.self"
	CodeBlacklistFull         = "blacklist.full"
)

type BlacklistUsecase struct {
	transactions repository.TransactionRunner
	repo         repository.BlacklistRepository
}

func NewBlacklistUsecase(
	transactions repository.TransactionRunner,
	repo repository.BlacklistRepository,
) *BlacklistUsecase {
	return &BlacklistUsecase{
		transactions: transactions,
		repo:         repo,
	}
}

type SetBlacklistCommand struct {
	BlockedUserID int64
	Blocked       bool
}

func checkBlacklistActor(actor Actor) error {
	if actor.UserID <= 0 {
		return PermissionDenied(CodeBlacklistActorInvalid, "无效的用户身份")
	}
	return nil
}

// List returns the complete bounded list in one snapshot (at most 1000 entries).
func (u *BlacklistUsecase) List(
	ctx context.Context,
	actor Actor,
) ([]domain.BlacklistEntry, error) {
	if err := checkBlacklistActor(actor); err != nil {
		return nil, err
	}
	return u.repo.List(ctx, actor.UserID)
}

func (u *BlacklistUsecase) Set(
	ctx context.Context,
	actor Actor,
	command SetBlacklistCommand,
) error {
	if err := checkBlacklistActor(actor); err != nil {
		return err
	}
	if command.BlockedUserID <= 0 {
		return Invalid(CodeBlacklistUserInvalid, "用户 ID 必须为正整数")
	}
	if command.BlockedUserID == actor.UserID {
		return Invalid(CodeBlacklistSelf, "不能拉黑自己")
	}
	return u.transactions.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := u.repo.LockUser(ctx, actor.UserID); err != nil {
			return err
		}
		if !command.Blocked {
			return u.repo.Remove(ctx, actor.UserID, command.BlockedUserID)
		}
		items, err := u.repo.List(ctx, actor.UserID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.BlockedUserID == command.BlockedUserID {
				return nil
			}
		}
		if len(items) >= BlacklistLimit {
			return Conflict(CodeBlacklistFull, "黑名单最多允许 1000 人")
		}
		return u.repo.Add(
			ctx,
			domain.BlacklistEntry{
				UserID:        actor.UserID,
				BlockedUserID: command.BlockedUserID,
			},
		)
	})
}
