//go:build integration

package tests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"forum/internal/domain"
	"forum/internal/infra"
	"forum/internal/repository"
	"forum/internal/usecase"
	"os"
	"strconv"
	"testing"
	"time"
)

var (
	testDB       *sql.DB
	tagRepo      repository.TagRepository
	postRepo     postFixture
	commentRepo  commentFixture
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
	transactions := repository.NewTransactionRunner(testDB)
	rawPosts := repository.NewPostRepository(testDB, tagRepo)
	rawComments := repository.NewCommentRepository(testDB)
	favoriteRepo = repository.NewFavoriteRepository(testDB)
	postRepo = postFixture{PostRepository: rawPosts, u: usecase.NewPostUsecase(transactions, rawPosts, tagRepo, favoriteRepo, nil)}
	commentRepo = commentFixture{CommentRepository: rawComments, u: usecase.NewCommentUsecase(transactions, rawComments, rawPosts, nil, fixtureSubjectResolver{})}
	resetDatabase()
	code := m.Run()
	resetDatabase()
	if err := testDB.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close integration database: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

// Existing integration cases exercise business writes through real usecases;
// reads still target the concrete repositories directly.
type fixtureCreatePostInput struct {
	CategoryID     int64
	Title, Content string
	AuthorID       int64
	AuthorUsername string
	TagIDs         []int64
	Attr           string
}
type postFixture struct {
	repository.PostRepository
	u *usecase.PostUsecase
}

func (r postFixture) Create(ctx context.Context, input fixtureCreatePostInput) (*domain.Post, error) {
	result, err := r.u.Create(ctx, usecase.Actor{UserID: input.AuthorID, Username: input.AuthorUsername, IsAdmin: true}, usecase.PostInput{CategoryID: input.CategoryID, Title: input.Title, Content: input.Content, TagIDs: input.TagIDs})
	if err != nil {
		return nil, err
	}
	return &result.Post, nil
}
func (r postFixture) Update(ctx context.Context, id int64, input usecase.PostInput) (*domain.Post, error) {
	result, err := r.u.Update(ctx, usecase.Actor{IsAdmin: true}, id, input)
	if err != nil {
		return nil, err
	}
	return &result.Post, nil
}

type commentFixture struct {
	repository.CommentRepository
	u *usecase.CommentUsecase
}

func (r commentFixture) Create(ctx context.Context, input domain.Comment) (*domain.Comment, error) {
	actor := usecase.Actor{UserID: input.AuthorID, Username: input.AuthorUsername, IsAdmin: true}
	if input.SubjectType == domain.CommentSubjectPost {
		id, err := domain.PostIDFromCommentSubjectKey(input.SubjectKey)
		if err != nil {
			return nil, err
		}
		return r.u.Create(ctx, actor, usecase.CreatePostCommentCommand{PostID: id, RootID: input.RootID, Content: input.Content})
	}
	return r.u.CreateExternal(ctx, actor, usecase.CreateExternalCommentCommand{Kind: "novel", SubjectKey: input.SubjectKey, RootID: input.RootID, Content: input.Content})
}
func (r commentFixture) Update(ctx context.Context, kind domain.CommentSubjectType, id int64, content string) (*domain.Comment, error) {
	return r.u.Update(ctx, usecase.Actor{IsAdmin: true}, usecase.UpdateCommentCommand{SubjectType: kind, CommentID: id, Content: content})
}
func (r commentFixture) SetStatus(ctx context.Context, kind domain.CommentSubjectType, id int64, status domain.CommentStatus) error {
	return r.u.SetStatus(ctx, usecase.Actor{IsAdmin: true}, usecase.SetCommentStatusCommand{SubjectType: kind, CommentID: id, Status: status})
}
func (r commentFixture) DeleteAllByAuthor(ctx context.Context, authorID int64) error {
	return r.u.DeleteAllByAuthor(ctx, usecase.Actor{IsAdmin: true}, usecase.DeleteCommentsByAuthorCommand{AuthorID: authorID})
}

// External resource availability is fixture data, not a copy of comment rules.
type fixtureSubjectResolver struct{}

func (fixtureSubjectResolver) Type(kind string) (domain.CommentSubjectType, bool) {
	return domain.CommentSubjectNovel, kind == "novel"
}
func (fixtureSubjectResolver) Valid(string, string) bool                           { return true }
func (fixtureSubjectResolver) Check(context.Context, string, string) (bool, error) { return true, nil }
func isAppErrorCode(err error, code string) bool {
	var appErr *usecase.AppError
	return errors.As(err, &appErr) && appErr.Code == code
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
