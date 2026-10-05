package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
)

// A canceled operation must stop before opening a connection. This driver makes
// the regression independent of a running PostgreSQL server and fails if used.
type contextOnlyDriver struct{}

func (contextOnlyDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("unexpected connection attempt for canceled operation")
}

func init() { sql.Register("forum-context-only", contextOnlyDriver{}) }

func TestRepositoriesRespectCanceledContext(t *testing.T) {
	db, err := sql.Open("forum-context-only", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tags := NewTagRepository(db)
	posts := NewPostRepository(db, tags)
	comments := NewCommentRepository(db)
	favorites := NewFavoriteRepository(db)
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"post list", func() error {
			_, _, err := posts.List(ctx, PostFilter{}, 20, 0)
			return err
		}},
		{"post find", func() error {
			_, err := posts.Find(ctx, 1, false)
			return err
		}},
		{"post view", func() error {
			_, err := posts.Find(ctx, 1, true)
			return err
		}},
		{"post exists", func() error {
			_, err := posts.ExistsPublished(ctx, 1)
			return err
		}},
		{"post create transaction", func() error {
			_, err := posts.Create(ctx, CreatePostInput{CategoryID: forumcategory.NovelID})
			return err
		}},
		{"post update transaction", func() error {
			_, err := posts.Update(ctx, 1, UpdatePostInput{CategoryID: forumcategory.NovelID})
			return err
		}},
		{"post status", func() error {
			return posts.SetStatus(ctx, 1, domain.PostStatusHidden)
		}},
		{"post lock", func() error {
			return posts.SetCommentsLocked(ctx, 1, true)
		}},
		{"post pin", func() error {
			return posts.SetPinOrder(ctx, 1, nil)
		}},
		{"comment admin", func() error {
			_, _, err := comments.ListAdmin(ctx, CommentFilter{}, 20, 0)
			return err
		}},
		{"comment roots", func() error {
			_, _, err := comments.ListRoots(ctx, domain.CommentSubjectNovel, "book", 20, 0)
			return err
		}},
		{"comment replies", func() error {
			_, _, err := comments.ListReplies(ctx, domain.CommentSubjectNovel, "book", 1, 20, 0)
			return err
		}},
		{"comment find", func() error {
			_, err := comments.Find(ctx, domain.CommentSubjectNovel, 1)
			return err
		}},
		{"comment create transaction", func() error {
			_, err := comments.Create(ctx, domain.Comment{SubjectType: domain.CommentSubjectNovel, SubjectKey: "book"})
			return err
		}},
		{"comment update", func() error {
			_, err := comments.Update(ctx, domain.CommentSubjectNovel, 1, "body")
			return err
		}},
		{"comment status transaction", func() error {
			return comments.SetStatus(ctx, domain.CommentSubjectNovel, 1, domain.CommentStatusHidden)
		}},
		{"comment status by author", func() error {
			_, err := comments.SetStatusByAuthor(ctx, 1, []domain.CommentStatus{domain.CommentStatusPublished, domain.CommentStatusHidden}, domain.CommentStatusDeleted)
			return err
		}},
		{"favorite has", func() error {
			_, err := favorites.Has(ctx, 1, 1)
			return err
		}},
		{"favorite list", func() error {
			_, err := favorites.ListPostIDs(ctx, 1, []int64{1})
			return err
		}},
		{"favorite add", func() error {
			return favorites.Set(ctx, 1, 1, true)
		}},
		{"favorite remove", func() error {
			return favorites.Set(ctx, 1, 1, false)
		}},
		{"tag category", func() error {
			_, err := tags.ListByCategory(ctx, 1)
			return err
		}},
		{"tag active", func() error {
			_, err := tags.ListActive(ctx)
			return err
		}},
		{"tag post", func() error {
			_, err := tags.ListForPost(ctx, 1)
			return err
		}},
		{"tag posts", func() error {
			_, err := tags.ListForPosts(ctx, []int64{1})
			return err
		}},
		{"tag create", func() error {
			_, err := tags.Create(ctx, 1, "tag", 1, 1, "{}")
			return err
		}},
		{"tag update", func() error {
			_, err := tags.Update(ctx, 1, 1, "tag", 1, 1)
			return err
		}},
		{"tag active update", func() error {
			return tags.SetActive(ctx, 1, 1, false)
		}},
		{"post tag helper", func() error {
			return posts.ReplaceTags(ctx, 1, []int64{1})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, context.Canceled) {
				t.Fatal(fmt.Errorf("cancellation cause lost: %w", err))
			}
		})
	}
}
