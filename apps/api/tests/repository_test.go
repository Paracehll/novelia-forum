//go:build integration

package tests

import (
	"errors"
	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/repository"
	"testing"
)

func TestJetRepositories(t *testing.T) {
	resetDatabase()
	category, _ := forumcategory.FindByID(forumcategory.NovelID)
	if _, err := postRepo.Create(repository.CreatePostInput{
		CategoryID: 999,
		Title:      "无效分类",
		Content:    "正文",
		AuthorID:   7,
		Attr:       `{}`,
	}); !errors.Is(err, repository.ErrInvalidCategory) {
		t.Fatalf("got %v, want ErrInvalidCategory", err)
	}
	tag, err := tagRepo.Create(category.ID, "公告", 1, 10, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tag.IsActive {
		t.Fatal("new tag is inactive")
	}
	otherCategory, _ := forumcategory.FindByID(forumcategory.AnnouncementsID)
	if _, err := tagRepo.Update(otherCategory.ID, tag.ID, "跨分类更新", 2, 20); !repository.IsNotFound(err) {
		t.Fatalf("cross-category update error = %v, want not found", err)
	}
	if err := tagRepo.SetActive(otherCategory.ID, tag.ID, false); !repository.IsNotFound(err) {
		t.Fatalf("cross-category activation error = %v, want not found", err)
	}
	if err := tagRepo.SetActive(tag.CategoryID, tag.ID, false); err != nil {
		t.Fatal(err)
	}
	tag, err = tagRepo.Update(tag.CategoryID, tag.ID, "公告", 2, 20)
	if err != nil {
		t.Fatal(err)
	}
	if tag.IsActive || tag.Color != 2 || tag.SortOrder != 20 {
		t.Fatalf("unexpected updated tag: %#v", tag)
	}
	if err := tagRepo.SetActive(tag.CategoryID, tag.ID, true); err != nil {
		t.Fatal(err)
	}
	inactiveTag, err := tagRepo.Create(category.ID, "停用标签", 3, 30, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := tagRepo.SetActive(inactiveTag.CategoryID, inactiveTag.ID, false); err != nil {
		t.Fatal(err)
	}
	activeTags, err := tagRepo.ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(activeTags) != 1 || activeTags[0].ID != tag.ID {
		t.Fatalf("unexpected active tags: %#v", activeTags)
	}
	categoryTags, err := tagRepo.ListByCategory(category.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(categoryTags) != 2 || categoryTags[0].ID != tag.ID || categoryTags[1].ID != inactiveTag.ID {
		t.Fatalf("unexpected category tags: %#v", categoryTags)
	}

	post, err := postRepo.Create(repository.CreatePostInput{
		CategoryID: category.ID,
		Title:      "第一篇帖子", Content: "正文", AuthorID: 7, AuthorUsername: "alice",
		TagIDs: []int64{tag.ID}, Attr: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(post.Tags) != 1 || post.Tags[0].ID != tag.ID {
		t.Fatalf("unexpected tags: %#v", post.Tags)
	}
	const otherSubjectType = domain.CommentSubjectNovel
	otherSubjectComment, err := commentRepo.Create(domain.Comment{
		SubjectType:    otherSubjectType,
		SubjectKey:     "novel:chapter-1",
		Content:        "其他主体评论",
		AuthorID:       8,
		AuthorUsername: "bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTotal, otherComments, err := commentRepo.ListRoots(otherSubjectType, "novel:chapter-1", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if otherTotal != 1 || len(otherComments) != 1 || otherComments[0].Root.SubjectKey != "novel:chapter-1" {
		t.Fatalf("unexpected external comments: total=%d items=%#v", otherTotal, otherComments)
	}
	var commentsCount int32
	if err := testDB.QueryRow("SELECT comments_count FROM post WHERE id = $1", post.ID).Scan(&commentsCount); err != nil {
		t.Fatal(err)
	}
	if commentsCount != 0 {
		t.Fatalf("post comments count changed after creating a non-post comment: %d", commentsCount)
	}
	if err := commentRepo.SetStatus(otherSubjectType, otherSubjectComment.ID, domain.CommentStatusDeleted); err != nil {
		t.Fatal(err)
	}
	otherTotal, otherComments, err = commentRepo.ListRoots(otherSubjectType, "novel:chapter-1", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if otherTotal != 1 || len(otherComments) != 1 || otherComments[0].Root.Status != domain.CommentStatusDeleted {
		t.Fatalf("deleted external comment missing: total=%d items=%#v", otherTotal, otherComments)
	}
	if err := testDB.QueryRow("SELECT comments_count FROM post WHERE id = $1", post.ID).Scan(&commentsCount); err != nil {
		t.Fatal(err)
	}
	if commentsCount != 0 {
		t.Fatalf("post comments count changed after moderating a non-post comment: %d", commentsCount)
	}

	total, posts, err := postRepo.List(repository.PostFilter{CategorySlug: category.Slug, TagIDs: []int64{tag.ID}}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(posts) != 1 {
		t.Fatalf("unexpected posts: total=%d items=%d", total, len(posts))
	}

	if posts[0].Content != "" || posts[0].Attr != "" {
		t.Fatal("list loaded content or attributes")
	}

	viewed, err := postRepo.Find(post.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if viewed.Content != post.Content {
		t.Fatal("detail did not preserve content")
	}
	if viewed.ViewsCount != 1 {
		t.Fatalf("views count = %d", viewed.ViewsCount)
	}

	root, err := commentRepo.Create(domain.Comment{
		SubjectType:    domain.CommentSubjectPost,
		SubjectKey:     domain.PostCommentSubjectKey(post.ID),
		Content:        "评论",
		AuthorID:       8,
		AuthorUsername: "bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.SubjectType != domain.CommentSubjectPost || root.SubjectKey != domain.PostCommentSubjectKey(post.ID) {
		t.Fatalf("unexpected comment subject: type=%d key=%s", root.SubjectType, root.SubjectKey)
	}
	var commentAttr string
	if err := testDB.QueryRow("SELECT attr FROM comment WHERE id = $1", root.ID).Scan(&commentAttr); err != nil {
		t.Fatal(err)
	}
	if commentAttr != "{}" {
		t.Fatalf("comment attr = %q, want repository default", commentAttr)
	}
	_, err = commentRepo.Create(domain.Comment{
		SubjectType:    domain.CommentSubjectPost,
		SubjectKey:     domain.PostCommentSubjectKey(post.ID),
		RootID:         &root.ID,
		Content:        "回复",
		AuthorID:       7,
		AuthorUsername: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	commentTotal, comments, err := commentRepo.ListRoots(domain.CommentSubjectPost, domain.PostCommentSubjectKey(post.ID), 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if commentTotal != 1 || len(comments) != 1 || comments[0].ReplyCount != 1 {
		t.Fatalf("unexpected comments: total=%d items=%d", commentTotal, len(comments))
	}
	replyTotal, replies, err := commentRepo.ListReplies(domain.CommentSubjectPost, domain.PostCommentSubjectKey(post.ID), root.ID, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if replyTotal != 1 || len(replies) != 1 || replies[0].RootID == nil || *replies[0].RootID != root.ID {
		t.Fatalf("unexpected replies: total=%d items=%#v", replyTotal, replies)
	}

	if err := favoriteRepo.Set(post.ID, 7, true); err != nil {
		t.Fatal(err)
	}
	favorited, err := favoriteRepo.Has(post.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !favorited {
		t.Fatal("favorite was not found after insertion")
	}
	favoriteIDs, err := favoriteRepo.ListPostIDs(7, []int64{post.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !favoriteIDs[post.ID] {
		t.Fatal("favorite post ID was not returned")
	}
	favoriteTotal, _, err := postRepo.List(repository.PostFilter{FavoriteUserID: 7}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if favoriteTotal != 1 {
		t.Fatalf("favorite total = %d", favoriteTotal)
	}

	if err := postRepo.SetCommentsLocked(post.ID, true); err != nil {
		t.Fatal(err)
	}
	_, err = commentRepo.Create(domain.Comment{
		SubjectType:    domain.CommentSubjectPost,
		SubjectKey:     domain.PostCommentSubjectKey(post.ID),
		Content:        "blocked",
		AuthorID:       9,
		AuthorUsername: "carol",
	})
	if !errors.Is(err, repository.ErrCommentsLocked) {
		t.Fatalf("got %v, want ErrCommentsLocked", err)
	}
	for _, status := range []domain.CommentStatus{domain.CommentStatusHidden, domain.CommentStatusDeleted} {
		if err := commentRepo.SetStatus(domain.CommentSubjectPost, root.ID, status); err != nil {
			t.Fatal(err)
		}
		total, items, err := commentRepo.ListRoots(domain.CommentSubjectPost, domain.PostCommentSubjectKey(post.ID), 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(items) != 1 || items[0].Root.ID != root.ID || items[0].Root.Status != status || items[0].ReplyCount != 1 {
			t.Fatalf("moderation changed root pagination: total=%d items=%#v", total, items)
		}
		replyTotal, replies, err := commentRepo.ListReplies(domain.CommentSubjectPost, domain.PostCommentSubjectKey(post.ID), root.ID, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if replyTotal != 1 || len(replies) != 1 || replies[0].Status != domain.CommentStatusPublished {
			t.Fatalf("moderation changed reply pagination: total=%d items=%#v", replyTotal, replies)
		}
	}
	updatedCategory, _ := forumcategory.FindByID(forumcategory.FeedbackID)
	updatedTag, err := tagRepo.Create(updatedCategory.ID, "建议", 4, 10, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := postRepo.Update(post.ID, repository.UpdatePostInput{
		CategoryID: updatedCategory.ID,
		Title:      "更新标题",
		Content:    "更新正文",
		TagIDs:     []int64{updatedTag.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "更新标题" || updated.CommentsCount != 1 || updated.CategoryID != updatedCategory.ID {
		t.Fatalf("unexpected updated post: %#v", updated.Post)
	}
	if len(updated.Tags) != 1 || updated.Tags[0].ID != updatedTag.ID {
		t.Fatalf("unexpected updated tags: %#v", updated.Tags)
	}

	if err := postRepo.SetStatus(post.ID, repository.StatusHidden); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		filter repository.PostFilter
		want   int64
	}{
		{"public", repository.PostFilter{}, 0},
		{"admin all", repository.PostFilter{Status: repository.PostStatusAll}, 1},
		{"admin hidden", repository.PostFilter{Status: repository.StatusHidden}, 1},
		{"admin deleted", repository.PostFilter{Status: repository.StatusDeleted}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			total, items, err := postRepo.List(tc.filter, 20, 0)
			if err != nil {
				t.Fatal(err)
			}
			if total != tc.want || int64(len(items)) != tc.want {
				t.Fatalf("total=%d items=%d, want=%d", total, len(items), tc.want)
			}
		})
	}
	if err := postRepo.SetStatus(post.ID, repository.StatusDeleted); err != nil {
		t.Fatal(err)
	}
	deletedTotal, deletedPosts, err := postRepo.List(repository.PostFilter{Status: repository.StatusDeleted}, 20, 0)
	if err != nil || deletedTotal != 1 || len(deletedPosts) != 1 || deletedPosts[0].ID != post.ID {
		t.Fatalf("deleted post filter: total=%d items=%#v err=%v", deletedTotal, deletedPosts, err)
	}
	if err := postRepo.SetStatus(post.ID, repository.StatusPublished); err != nil {
		t.Fatal(err)
	}
	if err := postRepo.SetCommentsLocked(post.ID, true); err != nil {
		t.Fatal(err)
	}
	pinOrder := int32(0)
	if err := postRepo.SetPinOrder(post.ID, &pinOrder); err != nil {
		t.Fatal(err)
	}
	moderated, err := postRepo.Find(post.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !moderated.CommentsLocked || moderated.PinOrder == nil || *moderated.PinOrder != pinOrder {
		t.Fatalf("unexpected post subresources: %#v", moderated.Post)
	}
	if err := postRepo.SetCommentsLocked(post.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := postRepo.SetPinOrder(post.ID, nil); err != nil {
		t.Fatal(err)
	}

	secondPost, err := postRepo.Create(repository.CreatePostInput{
		CategoryID: category.ID,
		Title:      "CaseSensitiveTitle", Content: "用于排序", AuthorID: 8, AuthorUsername: "bob",
		Attr: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, newestPosts, err := postRepo.List(repository.PostFilter{Sort: repository.PostSortNewest}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(newestPosts) < 2 || newestPosts[0].ID != secondPost.ID {
		t.Fatalf("unexpected newest order: %#v", newestPosts)
	}
	if len(newestPosts[0].Tags) != 0 {
		t.Fatalf("unexpected tags for untagged post: %#v", newestPosts[0].Tags)
	}
	if len(newestPosts[1].Tags) != 1 || newestPosts[1].Tags[0].ID != updatedTag.ID {
		t.Fatalf("unexpected batched tags: %#v", newestPosts[1].Tags)
	}
	_, viewedPosts, err := postRepo.List(repository.PostFilter{Sort: repository.PostSortViews}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(viewedPosts) < 2 || viewedPosts[0].ID != post.ID {
		t.Fatalf("unexpected views order: %#v", viewedPosts)
	}
	_, commentedPosts, err := postRepo.List(repository.PostFilter{Sort: repository.PostSortComments}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commentedPosts) < 2 || commentedPosts[0].ID != post.ID {
		t.Fatalf("unexpected comments order: %#v", commentedPosts)
	}
	searchTotal, searchedPosts, err := postRepo.List(repository.PostFilter{Search: "casesensitivetitle"}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if searchTotal != 1 || len(searchedPosts) != 1 || searchedPosts[0].ID != secondPost.ID {
		t.Fatalf("unexpected case-insensitive search: total=%d items=%#v", searchTotal, searchedPosts)
	}
}
