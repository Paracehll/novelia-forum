//go:build integration

package tests

import (
	"context"
	"fmt"
	"testing"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/repository"
	"forum/internal/usecase"
)

func TestBlacklistPostListVisibility(t *testing.T) {
	resetDatabase()
	ctx := context.Background()
	viewer := usecase.Actor{UserID: 10, IsAdmin: true}
	blacklist := usecase.NewBlacklistUsecase(
		repository.NewTransactionRunner(testDB), repository.NewBlacklistRepository(testDB),
	)
	var ids []int64
	for _, author := range []int64{20, 30, 20} {
		post, err := postRepo.Create(ctx, fixtureCreatePostInput{
			CategoryID: forumcategory.NovelID, Title: "黑名单帖子", Content: "正文",
			AuthorID: author, AuthorUsername: fmt.Sprint(author),
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, post.ID)
		if err := favoriteRepo.Set(ctx, post.ID, viewer.UserID, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := blacklist.Set(ctx, viewer, usecase.SetBlacklistCommand{BlockedUserID: 20, Blocked: true}); err != nil {
		t.Fatal(err)
	}
	// The reverse direction must not hide content: author 30 blocks the viewer.
	if err := blacklist.Set(ctx, usecase.Actor{UserID: 30}, usecase.SetBlacklistCommand{BlockedUserID: viewer.UserID, Blocked: true}); err != nil {
		t.Fatal(err)
	}
	query := usecase.ListPostsQuery{Limit: 1, Sort: domain.PostSortNewest, Search: "黑名单"}
	for _, list := range []struct {
		name string
		call func(context.Context, usecase.Actor, usecase.ListPostsQuery) (int64, []domain.PostListItem, error)
	}{{"public", postRepo.u.List}, {"favorites", postRepo.u.ListFavorites}} {
		t.Run(list.name, func(t *testing.T) {
			total, items, err := list.call(ctx, viewer, query)
			if err != nil || total != 1 || len(items) != 1 || items[0].ID != ids[1] {
				t.Fatalf("total=%d items=%v err=%v", total, items, err)
			}
			next := query
			next.Offset = 1
			total, items, err = list.call(ctx, viewer, next)
			if err != nil || total != 1 || len(items) != 0 {
				t.Fatalf("next page: %d %v %v", total, items, err)
			}
		})
	}
	for _, actor := range []usecase.Actor{{}, {UserID: 99}} {
		total, _, err := postRepo.u.List(ctx, actor, query)
		if err != nil || total != 3 {
			t.Fatalf("unaffected viewer: %d %v", total, err)
		}
	}
	total, _, err := postRepo.u.ListAdmin(ctx, viewer, query)
	if err != nil || total != 3 {
		t.Fatalf("admin list: %d %v", total, err)
	}
	if _, err := postRepo.u.Get(ctx, viewer, ids[0]); err != nil {
		t.Fatalf("direct post access: %v", err)
	}
	if err := blacklist.Set(ctx, viewer, usecase.SetBlacklistCommand{BlockedUserID: 20}); err != nil {
		t.Fatal(err)
	}
	total, _, err = postRepo.u.List(ctx, viewer, query)
	if err != nil || total != 3 {
		t.Fatalf("unblock: %d %v", total, err)
	}
}

func TestBlacklistCommentVisibility(t *testing.T) {
	for _, kind := range []domain.CommentSubjectType{domain.CommentSubjectPost, domain.CommentSubjectNovel} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			resetDatabase()
			ctx := context.Background()
			viewer := usecase.Actor{UserID: 10, IsAdmin: true}
			key := "blacklist-comments"
			if kind == domain.CommentSubjectPost {
				post, err := postRepo.Create(ctx, fixtureCreatePostInput{
					CategoryID: forumcategory.NovelID, Title: "评论测试", Content: "正文",
					AuthorID: 30, AuthorUsername: "author",
				})
				if err != nil {
					t.Fatal(err)
				}
				key = domain.PostCommentSubjectKey(post.ID)
			}
			create := func(author int64, root *int64) *domain.Comment {
				t.Helper()
				item, err := commentRepo.Create(ctx, domain.Comment{
					SubjectType: kind, SubjectKey: key, RootID: root,
					AuthorID: author, AuthorUsername: fmt.Sprint(author), Content: "评论",
				})
				if err != nil {
					t.Fatal(err)
				}
				return item
			}
			visibleRoot := create(30, nil)
			hiddenRoot := create(20, nil)
			create(30, &hiddenRoot.ID)
			// Blocked early replies must not occupy preview slots or pagination offsets.
			for i := 0; i < 3; i++ {
				create(20, &visibleRoot.ID)
			}
			var replies []int64
			for i := 0; i < 22; i++ {
				replies = append(replies, create(30, &visibleRoot.ID).ID)
			}
			blacklist := usecase.NewBlacklistUsecase(
				repository.NewTransactionRunner(testDB), repository.NewBlacklistRepository(testDB),
			)
			if err := blacklist.Set(ctx, viewer, usecase.SetBlacklistCommand{BlockedUserID: 20, Blocked: true}); err != nil {
				t.Fatal(err)
			}
			list := func(actor usecase.Actor) (int64, []domain.CommentThreadPreview, error) {
				if kind == domain.CommentSubjectNovel {
					return commentRepo.u.ListExternal(ctx, actor, usecase.ListExternalCommentsQuery{
						Kind: "novel", SubjectKey: key, Limit: 1,
					})
				}
				return commentRepo.u.List(ctx, actor, usecase.ListCommentsQuery{SubjectType: kind, SubjectKey: key, Limit: 1})
			}
			listReplies := func(root, offset int64) (int64, []domain.Comment, error) {
				if kind == domain.CommentSubjectNovel {
					return commentRepo.u.ListExternalReplies(ctx, viewer, usecase.ListExternalCommentRepliesQuery{
						Kind: "novel", SubjectKey: key, RootID: root, Limit: 20, Offset: offset,
					})
				}
				return commentRepo.u.ListReplies(ctx, viewer, usecase.ListCommentRepliesQuery{
					SubjectType: kind, SubjectKey: key, RootID: root, Limit: 20, Offset: offset,
				})
			}
			total, threads, err := list(viewer)
			if err != nil || total != 1 || len(threads) != 1 || threads[0].Root.ID != visibleRoot.ID {
				t.Fatalf("roots: %d %v %v", total, threads, err)
			}
			thread := threads[0]
			if thread.ReplyCount != 22 || len(thread.Replies) != 20 {
				t.Fatalf("preview: %+v", thread)
			}
			for i, reply := range thread.Replies {
				if reply.ID != replies[i] {
					t.Fatalf("preview[%d]=%d want=%d", i, reply.ID, replies[i])
				}
			}
			total, page, err := listReplies(visibleRoot.ID, 20)
			if err != nil || total != 22 || len(page) != 2 || page[0].ID != replies[20] {
				t.Fatalf("reply page: %d %v %v", total, page, err)
			}
			_, _, err = listReplies(hiddenRoot.ID, 0)
			if !isAppErrorCode(err, usecase.CodeCommentRootNotFound) {
				t.Fatalf("hidden root accessible: %v", err)
			}
			for _, actor := range []usecase.Actor{{}, {UserID: 99}} {
				total, _, err := list(actor)
				if err != nil || total != 2 {
					t.Fatalf("unaffected viewer: %d %v", total, err)
				}
			}
			if kind == domain.CommentSubjectPost {
				total, _, err := commentRepo.u.ListAdmin(ctx, viewer, usecase.ListAdminCommentsQuery{Limit: 100})
				if err != nil || total != 28 {
					t.Fatalf("admin comments: %d %v", total, err)
				}
			}
			if err := blacklist.Set(ctx, viewer, usecase.SetBlacklistCommand{BlockedUserID: 20}); err != nil {
				t.Fatal(err)
			}
			total, _, err = list(viewer)
			if err != nil || total != 2 {
				t.Fatalf("unblock: %d %v", total, err)
			}
		})
	}
}
