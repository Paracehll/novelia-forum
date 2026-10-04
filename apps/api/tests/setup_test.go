//go:build integration

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"forum/internal/domain"
	"forum/internal/infra"
	"forum/internal/repository"
	"os"
	"strconv"
	"testing"
	"time"
)

var (
	testDB       *sql.DB
	tagRepo      repository.TagRepository
	postRepo     repository.PostRepository
	commentRepo  repository.CommentRepository
	favoriteRepo repository.FavoriteRepository
)

func TestMain(m *testing.M) {
	testDB = infra.NewSQLDB(
		env("TEST_DB_HOST", "localhost"),
		envInt("TEST_DB_PORT", 5003),
		env("TEST_DB_USER", "forum"),
		env("TEST_DB_PASSWORD", "forum-test-password"),
		env("TEST_DB_NAME", "forum_test"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := testDB.PingContext(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration database is unavailable: %v\n", err)
		fmt.Fprintln(os.Stderr, "run from the repository root: ./apps/api/tests/run.sh")
		os.Exit(1)
	}
	tagRepo = repository.NewTagRepository(testDB)
	transactions := repository.NewTransactionManager(testDB)
	postRepo = transactionalPostRepository{
		PostRepository: repository.NewPostRepository(testDB, tagRepo), tx: transactions,
	}
	commentRepo = transactionalCommentRepository{
		CommentRepository: repository.NewCommentRepository(testDB), tx: transactions,
	}
	favoriteRepo = repository.NewFavoriteRepository(testDB)
	resetDatabase()
	code := m.Run()
	resetDatabase()
	if err := testDB.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close integration database: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

// The existing integration cases call repositories directly. Keep those calls
// atomic without moving transaction ownership back into production repositories.
type transactionalPostRepository struct {
	repository.PostRepository
	tx repository.TransactionRunner
}

func (r transactionalPostRepository) Create(ctx context.Context, input repository.CreatePostInput) (*domain.Post, error) {
	var post *domain.Post
	err := r.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		post, err = r.PostRepository.Create(txCtx, input)
		return err
	})
	if err != nil {
		return nil, err
	}
	return post, nil
}

func (r transactionalPostRepository) Update(ctx context.Context, id int64, input repository.UpdatePostInput) (*domain.Post, error) {
	var post *domain.Post
	err := r.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		post, err = r.PostRepository.Update(txCtx, id, input)
		return err
	})
	if err != nil {
		return nil, err
	}
	return post, nil
}

type transactionalCommentRepository struct {
	repository.CommentRepository
	tx repository.TransactionRunner
}

func (r transactionalCommentRepository) Create(ctx context.Context, input domain.Comment) (*domain.Comment, error) {
	var comment *domain.Comment
	err := r.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		comment, err = r.CommentRepository.Create(txCtx, input)
		return err
	})
	if err != nil {
		return nil, err
	}
	return comment, nil
}

func (r transactionalCommentRepository) SetStatus(ctx context.Context, subjectType domain.CommentSubjectType, id int64, status domain.CommentStatus) error {
	return r.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		return r.CommentRepository.SetStatus(txCtx, subjectType, id, status)
	})
}

func resetDatabase() {
	if _, err := testDB.Exec("TRUNCATE post_favorite, post_tag, comment, post, tag RESTART IDENTITY"); err != nil {
		panic(err)
	}
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return value
}
