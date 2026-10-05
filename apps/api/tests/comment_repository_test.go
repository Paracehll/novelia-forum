//go:build integration

package tests

import (
	"context"
	"fmt"
	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/usecase"
	"testing"
)

func TestCommentRepositoryDeleteAllByAuthor(t *testing.T) {
	resetDatabase()
	category, _ := forumcategory.FindByID(forumcategory.NovelID)
	firstPost, err := postRepo.Create(context.Background(), fixtureCreatePostInput{
		CategoryID: category.ID, Title: "第一篇帖子", Content: "正文",
		AuthorID: 1, AuthorUsername: "author", Attr: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondPost, err := postRepo.Create(context.Background(), fixtureCreatePostInput{
		CategoryID: category.ID, Title: "第二篇帖子", Content: "正文",
		AuthorID: 1, AuthorUsername: "author", Attr: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	createComment := func(subjectType domain.CommentSubjectType, subjectKey string, authorID int64) *domain.Comment {
		t.Helper()
		comment, err := commentRepo.Create(context.Background(), domain.Comment{
			SubjectType: subjectType, SubjectKey: subjectKey, Content: "评论",
			AuthorID: authorID, AuthorUsername: "commenter",
		})
		if err != nil {
			t.Fatal(err)
		}
		return comment
	}

	firstPublished := createComment(domain.CommentSubjectPost, domain.PostCommentSubjectKey(firstPost.ID), 7)
	firstHidden := createComment(domain.CommentSubjectPost, domain.PostCommentSubjectKey(firstPost.ID), 7)
	if err := commentRepo.SetStatus(
		context.Background(),
		domain.CommentSubjectPost,
		firstHidden.ID,
		domain.CommentStatusHidden,
	); err != nil {
		t.Fatal(err)
	}
	secondPublished := createComment(domain.CommentSubjectPost, domain.PostCommentSubjectKey(secondPost.ID), 7)
	external := createComment(domain.CommentSubjectNovel, "novel:chapter-1", 7)
	otherAuthor := createComment(domain.CommentSubjectPost, domain.PostCommentSubjectKey(firstPost.ID), 8)

	if err := commentRepo.DeleteAllByAuthor(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	// 重复删除必须保持幂等，不能再次扣减帖子评论数。
	if err := commentRepo.DeleteAllByAuthor(context.Background(), 7); err != nil {
		t.Fatal(err)
	}

	for subjectType, commentIDs := range map[domain.CommentSubjectType][]int64{
		domain.CommentSubjectPost:  {firstPublished.ID, firstHidden.ID, secondPublished.ID},
		domain.CommentSubjectNovel: {external.ID},
	} {
		for _, commentID := range commentIDs {
			comment, err := commentRepo.Find(context.Background(), subjectType, commentID)
			if err != nil {
				t.Fatal(err)
			}
			if comment.Status != domain.CommentStatusDeleted {
				t.Fatalf("comment %d status = %d", comment.ID, comment.Status)
			}
		}
	}

	remaining, err := commentRepo.Find(context.Background(), domain.CommentSubjectPost, otherAuthor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if remaining.Status != domain.CommentStatusPublished {
		t.Fatalf("other author's comment status = %d", remaining.Status)
	}

	first, err := postRepo.Find(context.Background(), firstPost.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := postRepo.Find(context.Background(), secondPost.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.CommentsCount != 1 || second.CommentsCount != 0 {
		t.Fatalf("unexpected comment counts: first=%d second=%d", first.CommentsCount, second.CommentsCount)
	}
}

func TestCommentRootReplyPreviews(t *testing.T) {
	resetDatabase()
	create := func(key string, rootID *int64) *domain.Comment {
		t.Helper()
		c, err := commentRepo.Create(context.Background(), domain.Comment{
			SubjectType: domain.CommentSubjectNovel, SubjectKey: key, RootID: rootID,
			Content: "reply", AuthorID: 1, AuthorUsername: "reader",
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	roots := []*domain.Comment{create("preview", nil), create("preview", nil), create("preview", nil)}
	for _, root := range roots[:2] {
		for i := 0; i < 23; i++ {
			create("preview", &root.ID)
		}
	}
	other := create("other", nil)
	create("other", &other.ID)
	total, threads, err := commentRepo.ListRoots(context.Background(), domain.CommentSubjectNovel, "preview", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(threads) != 3 {
		t.Fatalf("unexpected roots: %d %d", total, len(threads))
	}
	for i, thread := range threads {
		if thread.Root.ID != roots[len(roots)-1-i].ID {
			t.Fatalf("roots are not newest first: %#v", thread.Root)
		}
		if i == 0 {
			if thread.ReplyCount != 0 || len(thread.Replies) != 0 {
				t.Fatal("empty root has replies")
			}
			continue
		}
		count, replies, err := commentRepo.ListReplies(
			context.Background(),
			domain.CommentSubjectNovel,
			"preview",
			thread.Root.ID,
			20,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		if thread.ReplyCount != count || count != 23 || len(thread.Replies) != 20 {
			t.Fatalf("bad preview size/count: %#v", thread)
		}
		for j, reply := range thread.Replies {
			if j > 0 && (reply.CreatedAt.After(thread.Replies[j-1].CreatedAt) ||
				(reply.CreatedAt.Equal(thread.Replies[j-1].CreatedAt) && reply.ID >= thread.Replies[j-1].ID)) {
				t.Fatalf("replies are not newest first: %#v", thread.Replies)
			}
			if reply.ID != replies[j].ID || reply.RootID == nil || *reply.RootID != thread.Root.ID {
				t.Fatalf("preview order/group mismatch: %#v", reply)
			}
		}
	}
	_, page, err := commentRepo.ListRoots(context.Background(), domain.CommentSubjectNovel, "preview", 1, 1)
	if err != nil || len(page) != 1 || page[0].Root.ID != roots[1].ID || len(page[0].Replies) != 20 {
		t.Fatalf("root pagination: %#v %v", page, err)
	}
	_, empty, err := commentRepo.ListRoots(context.Background(), domain.CommentSubjectNovel, "preview", 3, 3)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty page: %#v %v", empty, err)
	}
}

func TestCommentCreationRootErrors(t *testing.T) {
	resetDatabase()
	post, err := postRepo.Create(context.Background(), fixtureCreatePostInput{
		CategoryID: forumcategory.NovelID, Title: "帖子", Content: "正文",
		AuthorID: 1, AuthorUsername: "author", Attr: "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherPost, err := postRepo.Create(context.Background(), fixtureCreatePostInput{
		CategoryID: forumcategory.NovelID, Title: "另一帖子", Content: "正文",
		AuthorID: 1, AuthorUsername: "author", Attr: "{}",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, subjectType := range []domain.CommentSubjectType{domain.CommentSubjectPost, domain.CommentSubjectNovel} {
		t.Run(fmt.Sprintf("subject=%d", subjectType), func(t *testing.T) {
			key := domain.PostCommentSubjectKey(post.ID)
			create := func(kind domain.CommentSubjectType, subjectKey string, rootID *int64) *domain.Comment {
				t.Helper()
				comment, err := commentRepo.Create(context.Background(), domain.Comment{
					SubjectType: kind, SubjectKey: subjectKey, RootID: rootID,
					Content: "评论", AuthorID: 1, AuthorUsername: "author",
				})
				if err != nil {
					t.Fatal(err)
				}
				return comment
			}
			root := create(subjectType, key, nil)
			reply := create(subjectType, key, &root.ID)
			otherSubject := create(subjectType, domain.PostCommentSubjectKey(otherPost.ID), nil)
			otherType := create(1-subjectType, key, nil)
			hidden := create(subjectType, key, nil)
			deleted := create(subjectType, key, nil)
			if err := commentRepo.SetStatus(
				context.Background(),
				subjectType,
				hidden.ID,
				domain.CommentStatusHidden,
			); err != nil {
				t.Fatal(err)
			}
			if err := commentRepo.SetStatus(
				context.Background(),
				subjectType,
				deleted.ID,
				domain.CommentStatusDeleted,
			); err != nil {
				t.Fatal(err)
			}
			before, err := postRepo.Find(context.Background(), post.ID)
			if err != nil {
				t.Fatal(err)
			}
			var countBefore int
			if err := testDB.QueryRow("SELECT COUNT(*) FROM comment").Scan(&countBefore); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				rootID int64
				want   string
			}{
				{"missing", 999999, usecase.CodeCommentRootNotFound},
				{"hidden", hidden.ID, usecase.CodeCommentRootNotFound},
				{"deleted", deleted.ID, usecase.CodeCommentRootNotFound},
				{"reply as root", reply.ID, usecase.CodeCommentRootInvalid},
				{"other type", otherType.ID, usecase.CodeCommentRootNotFound},
				{"other subject", otherSubject.ID, usecase.CodeCommentRootInvalid},
			} {
				t.Run(tc.name, func(t *testing.T) {
					_, err := commentRepo.Create(context.Background(), domain.Comment{
						SubjectType: subjectType, SubjectKey: key, RootID: &tc.rootID,
						Content: "reply", AuthorID: 1, AuthorUsername: "author",
					})
					if !isAppErrorCode(err, tc.want) {
						t.Fatalf("got %v, want %v", err, tc.want)
					}
				})
			}
			var countAfter int
			if err := testDB.QueryRow("SELECT COUNT(*) FROM comment").Scan(&countAfter); err != nil {
				t.Fatal(err)
			}
			after, err := postRepo.Find(context.Background(), post.ID)
			if err != nil {
				t.Fatal(err)
			}
			if countBefore != countAfter || before.CommentsCount != after.CommentsCount {
				t.Fatal("failed creation changed comments or post count")
			}
		})
	}
}
