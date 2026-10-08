package usecase

import (
	"context"
	"reflect"
	"testing"
	"time"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/repository"
)

// immediateTransaction preserves existing unit-test contexts and repository stubs.
type immediateTransaction struct{}

func (immediateTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type contextRecorder struct {
	t     *testing.T
	want  context.Context
	calls []string
}

func (r *contextRecorder) record(ctx context.Context, operation string) {
	r.t.Helper()
	if ctx != r.want {
		r.t.Fatalf("%s replaced the caller context", operation)
	}
	r.calls = append(r.calls, operation)
}

func (r *contextRecorder) assertCalls(want ...string) {
	r.t.Helper()
	if !reflect.DeepEqual(r.calls, want) {
		r.t.Fatalf("calls = %v, want %v", r.calls, want)
	}
}

type contextPostRepository struct {
	repository.PostRepository
	recorder *contextRecorder
}

func (r contextPostRepository) List(
	ctx context.Context,
	_ repository.PostFilter,
	_, _ int64,
) (int64, []domain.PostListItem, error) {
	r.recorder.record(ctx, "post.list")
	return 1, []domain.PostListItem{{ID: 7}}, nil
}
func (r contextPostRepository) Find(ctx context.Context, _ int64) (*domain.Post, error) {
	r.recorder.record(ctx, "post.find")
	return &domain.Post{ID: 7, AuthorID: 1, CreatedAt: time.Now()}, nil
}
func (r contextPostRepository) IncrementViews(ctx context.Context, _ int64) (int32, error) {
	r.recorder.record(ctx, "post.increment_views")
	return 1, nil
}
func (r contextPostRepository) Lock(ctx context.Context, _ int64) (*domain.Post, error) {
	r.recorder.record(ctx, "post.lock")
	return &domain.Post{ID: 7, AuthorID: 1, CreatedAt: time.Now()}, nil
}
func (r contextPostRepository) ReplaceTags(ctx context.Context, _ int64, _ []int64) error {
	r.recorder.record(ctx, "post.replace_tags")
	return nil
}
func (r contextPostRepository) AdjustCommentsCount(ctx context.Context, _ int64, _ int32, _ *time.Time) error {
	r.recorder.record(ctx, "post.adjust_comments")
	return nil
}

func (r contextPostRepository) Update(
	ctx context.Context,
	_ int64,
	_ repository.UpdatePostInput,
) (*domain.Post, error) {
	r.recorder.record(ctx, "post.update")
	return &domain.Post{ID: 7}, nil
}
func (r contextPostRepository) SetStatus(ctx context.Context, _ int64, _ domain.PostStatus) error {
	r.recorder.record(ctx, "post.status")
	return nil
}
func (r contextPostRepository) ExistsPublished(ctx context.Context, _ int64) (bool, error) {
	r.recorder.record(ctx, "post.exists")
	return true, nil
}

type contextFavoriteRepository struct {
	repository.FavoriteRepository
	recorder *contextRecorder
}

func (r contextFavoriteRepository) ListPostIDs(ctx context.Context, _ int64, _ []int64) (map[int64]bool, error) {
	r.recorder.record(ctx, "favorite.list")
	return map[int64]bool{7: true}, nil
}
func (r contextFavoriteRepository) Has(ctx context.Context, _, _ int64) (bool, error) {
	r.recorder.record(ctx, "favorite.has")
	return true, nil
}

func TestPostContextSurvivesHelpersAndMultipleRepositories(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	actor := Actor{UserID: 1}
	for _, tc := range []struct {
		name string
		call func(*PostUsecase) error
		want []string
	}{
		{"list", func(u *PostUsecase) error {
			_, _, err := u.List(ctx, actor, ListPostsQuery{Limit: 20})
			return err
		}, []string{"post.list", "favorite.list"}},
		{"mine", func(u *PostUsecase) error {
			_, _, err := u.ListMine(ctx, actor, ListPostsQuery{Limit: 20})
			return err
		}, []string{"post.list", "favorite.list"}},
		{"favorites", func(u *PostUsecase) error {
			_, _, err := u.ListFavorites(ctx, actor, ListPostsQuery{Limit: 20})
			return err
		}, []string{"post.list", "favorite.list"}},
		{"get", func(u *PostUsecase) error {
			_, err := u.Get(ctx, actor, 7)
			return err
		}, []string{"post.find", "favorite.has", "post.increment_views"}},
		{"update", func(u *PostUsecase) error {
			_, err := u.Update(ctx, actor, 7, PostInput{CategoryID: forumcategory.NovelID, Title: "标题", Content: "body"})
			return err
		}, []string{"post.lock", "post.update", "post.replace_tags", "favorite.has"}},
		{"delete", func(u *PostUsecase) error {
			return u.Delete(ctx, actor, 7)
		}, []string{"post.lock", "post.status"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &contextRecorder{t: t, want: ctx}
			u := NewPostUsecase(immediateTransaction{}, contextPostRepository{recorder: r}, &postTagRepoStub{}, contextFavoriteRepository{recorder: r}, nil)
			if err := tc.call(u); err != nil {
				t.Fatal(err)
			}
			r.assertCalls(tc.want...)
		})
	}
}

type contextCommentRepository struct {
	repository.CommentRepository
	recorder *contextRecorder
}

func (r contextCommentRepository) ListRoots(
	ctx context.Context,
	_ domain.CommentSubjectType,
	_ string,
	_, _ int64,
	_ int64,
) (int64, []domain.CommentThreadPreview, error) {
	r.recorder.record(ctx, "comment.list")
	return 0, nil, nil
}
func (r contextCommentRepository) Create(ctx context.Context, _ domain.Comment) (*domain.Comment, error) {
	r.recorder.record(ctx, "comment.create")
	return &domain.Comment{}, nil
}

func TestCommentContextSurvivesSubjectCheckAndWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Run("published post check and read", func(t *testing.T) {
		r := &contextRecorder{t: t, want: ctx}
		u := NewCommentUsecase(immediateTransaction{}, contextCommentRepository{recorder: r}, contextPostRepository{recorder: r}, nil, nil)
		if _, _, err := u.List(
			ctx,
			Actor{},
			ListCommentsQuery{
				SubjectType: domain.CommentSubjectPost,
				SubjectKey:  "7",
				Limit:       20,
			},
		); err != nil {
			t.Fatal(err)
		}
		r.assertCalls("post.exists", "comment.list")
	})
	t.Run("external check and write", func(t *testing.T) {
		r := &contextRecorder{t: t, want: ctx}
		resolver := subjectCheckFunc(func(checkCtx context.Context, _, _ string) bool {
			r.record(checkCtx, "subject.check")
			return true
		})
		u := NewCommentUsecase(immediateTransaction{}, contextCommentRepository{recorder: r}, nil, nil, resolver)
		if _, err := u.CreateExternal(
			ctx,
			Actor{UserID: 1, CreatedAt: time.Now().Add(-30 * 24 * time.Hour)},
			CreateExternalCommentCommand{
				Kind:       "novel",
				SubjectKey: "book",
				Content:    "body",
			},
		); err != nil {
			t.Fatal(err)
		}
		r.assertCalls("subject.check", "comment.create")
	})
}
