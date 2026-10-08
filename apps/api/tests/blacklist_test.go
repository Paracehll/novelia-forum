//go:build integration

package tests

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"forum/internal/repository"
	"forum/internal/usecase"
)

func TestBlacklistLifecycleAndConcurrentLimit(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewBlacklistRepository(testDB)
	u := usecase.NewBlacklistUsecase(repository.NewTransactionRunner(testDB), repo)
	actor := usecase.Actor{UserID: 900001}
	t.Cleanup(func() { _, _ = testDB.Exec(`DELETE FROM user_blacklist`) })
	set := func(id int64, blocked bool) error {
		return u.Set(ctx, actor, usecase.SetBlacklistCommand{BlockedUserID: id, Blocked: blocked})
	}
	for _, id := range []int64{0, -1, actor.UserID} {
		if err := set(id, true); err == nil {
			t.Fatalf("accepted invalid target %d", id)
		}
	}
	if err := set(1, true); err != nil {
		t.Fatal(err)
	}
	first, err := u.List(ctx, actor)
	if err != nil || len(first) != 1 {
		t.Fatalf("list=%v err=%v", first, err)
	}
	if err := set(1, true); err != nil {
		t.Fatal(err)
	}
	second, err := u.List(ctx, actor)
	if err != nil || len(second) != 1 || !second[0].CreatedAt.Equal(first[0].CreatedAt) {
		t.Fatalf("duplicate changed entry: %v %v", second, err)
	}
	other := usecase.Actor{UserID: 900002}
	if err := u.Set(ctx, other, usecase.SetBlacklistCommand{BlockedUserID: 1}); err != nil {
		t.Fatal(err)
	}
	items, err := u.List(ctx, other)
	if err != nil || len(items) != 0 {
		t.Fatalf("owner isolation: %v %v", items, err)
	}
	items, err = u.List(ctx, actor)
	if err != nil || len(items) != 1 {
		t.Fatalf("other owner removed entry: %v %v", items, err)
	}
	if _, err := testDB.Exec(`INSERT INTO user_blacklist (user_id, blocked_user_id) SELECT $1, generate_series(2, 999)`, actor.UserID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := int64(1000); i < 1020; i++ {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); results <- set(id, true) }(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !isAppErrorCode(err, usecase.CodeBlacklistFull) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful additions=%d, want 1", successes)
	}
	items, err = u.List(ctx, actor)
	if err != nil || len(items) != 1000 {
		t.Fatalf("count=%d err=%v", len(items), err)
	}
	if err := set(1, true); err != nil {
		t.Fatalf("duplicate at capacity: %v", err)
	}
	if err := set(1, false); err != nil {
		t.Fatal(err)
	}
	if err := set(1, false); err != nil {
		t.Fatal(err)
	}
	if err := set(2000, true); err != nil {
		t.Fatalf("reuse capacity: %v", err)
	}
	// A failed usecase transaction must release the lock and roll back changes.
	sentinel := fmt.Errorf("rollback")
	err = repository.NewTransactionRunner(testDB).WithinTransaction(ctx, func(ctx context.Context) error {
		if err := repo.LockUser(ctx, actor.UserID); err != nil {
			return err
		}
		if err := repo.Remove(ctx, actor.UserID, 2000); err != nil {
			return err
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatal(err)
	}
	if err := set(2001, true); !isAppErrorCode(err, usecase.CodeBlacklistFull) {
		t.Fatalf("rollback lost entry: %v", err)
	}
}
