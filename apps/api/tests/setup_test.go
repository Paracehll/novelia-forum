//go:build integration

package tests

import (
	"context"
	"database/sql"
	"fmt"
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
	postRepo = repository.NewPostRepository(testDB, tagRepo)
	commentRepo = repository.NewCommentRepository(testDB)
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
