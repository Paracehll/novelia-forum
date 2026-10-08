//go:build integration

package tests

import (
	"context"
	"fmt"
	"sort"
	"testing"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/repository"
)

// A list page resolves tags for every post in one batched query. Each post must
// keep its own tags, including tags shared with other posts on the same page.
func TestPostListKeepsEveryTagPerPost(t *testing.T) {
	resetDatabase()
	ctx := context.Background()
	category, _ := forumcategory.FindByID(forumcategory.NovelID)

	tagIDs := make([]int64, 0, 5)
	for index, name := range []string{"书单", "工具", "讨论", "公告", "求助"} {
		tag, err := tagRepo.Create(ctx, category.ID, name, int16(index), int32((index+1)*10), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		tagIDs = append(tagIDs, tag.ID)
	}

	want := make(map[int64][]int64)
	for index := 0; index < 5; index++ {
		// Overlapping three-tag sets, so most tags are shared across posts.
		assigned := []int64{tagIDs[index], tagIDs[(index+2)%5], tagIDs[(index+4)%5]}
		post, err := postRepo.Create(ctx, fixtureCreatePostInput{
			CategoryID:     category.ID,
			Title:          fmt.Sprintf("帖子-%d", index),
			Content:        "正文",
			AuthorID:       int64(7 + index),
			AuthorUsername: "alice",
			TagIDs:         assigned,
			Attr:           `{}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		want[post.ID] = assigned
	}
	untagged, err := postRepo.Create(ctx, fixtureCreatePostInput{
		CategoryID: category.ID, Title: "无标签帖子", Content: "正文",
		AuthorID: 20, AuthorUsername: "bob", Attr: `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	want[untagged.ID] = nil

	_, items, err := postRepo.List(ctx, repository.PostFilter{Sort: domain.PostSortNewest}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(want) {
		t.Fatalf("list returned %d items, want %d", len(items), len(want))
	}
	for _, item := range items {
		got := make([]int64, len(item.Tags))
		for index, tag := range item.Tags {
			got[index] = tag.ID
		}
		expected := append([]int64(nil), want[item.ID]...)
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
		if fmt.Sprint(got) != fmt.Sprint(expected) {
			t.Errorf("post %d tags = %v, want %v", item.ID, got, expected)
		}
	}
}

// Filtering the page by one tag must not strip the remaining tags from the item.
func TestPostListTagFilterKeepsEveryTag(t *testing.T) {
	resetDatabase()
	ctx := context.Background()
	category, _ := forumcategory.FindByID(forumcategory.NovelID)

	first, err := tagRepo.Create(ctx, category.ID, "书单", 0, 10, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tagRepo.Create(ctx, category.ID, "工具", 1, 20, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := postRepo.Create(ctx, fixtureCreatePostInput{
		CategoryID: category.ID, Title: "双标签帖子", Content: "正文",
		AuthorID: 7, AuthorUsername: "alice",
		TagIDs: []int64{first.ID, second.ID}, Attr: `{}`,
	}); err != nil {
		t.Fatal(err)
	}

	for _, filterTag := range []int64{first.ID, second.ID} {
		_, items, err := postRepo.List(ctx, repository.PostFilter{
			Sort: domain.PostSortNewest, TagIDs: []int64{filterTag},
		}, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 {
			t.Fatalf("filter tag %d returned %d items, want 1", filterTag, len(items))
		}
		if len(items[0].Tags) != 2 {
			t.Errorf("filter tag %d item tags = %#v, want both tags", filterTag, items[0].Tags)
		}
	}
}
