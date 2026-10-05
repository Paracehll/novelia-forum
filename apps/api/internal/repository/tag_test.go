package repository

import (
	"context"
	"reflect"
	"testing"
	"time"

	"forum/.gen/main/public/model"
	"forum/internal/domain"
)

func TestTagFromModel(t *testing.T) {
	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(time.Hour)
	stored := model.Tag{
		ID: 12, CategoryID: 34, Name: "tag", Color: 2, IsActive: true,
		SortOrder: 7, CreatedAt: created, UpdatedAt: updated, Attr: "metadata",
	}
	want := domain.Tag{
		ID: 12, CategoryID: 34, Name: "tag", Color: 2, IsActive: true,
		SortOrder: 7, CreatedAt: created, UpdatedAt: updated, Attr: "metadata",
	}
	if got := tagFromModel(stored); !reflect.DeepEqual(got, want) {
		t.Fatalf("tagFromModel() = %#v, want %#v", got, want)
	}
}

func TestTagsFromModels(t *testing.T) {
	if got := tagsFromModels(nil); got != nil {
		t.Fatalf("nil input = %#v, want nil", got)
	}
	if got := tagsFromModels([]model.Tag{}); got == nil || len(got) != 0 {
		t.Fatalf("empty input = %#v, want non-nil empty slice", got)
	}
	stored := []model.Tag{{ID: 2, Name: "first"}, {ID: 1, Name: "second"}}
	want := []domain.Tag{{ID: 2, Name: "first"}, {ID: 1, Name: "second"}}
	if got := tagsFromModels(stored); !reflect.DeepEqual(got, want) {
		t.Fatalf("tagsFromModels() = %#v, want %#v", got, want)
	}
}

func TestTagListForPostsEmpty(t *testing.T) {
	repo := NewTagRepository(nil)
	got, err := repo.ListForPosts(context.Background(), nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("ListForPosts(nil) = %#v, %v; want non-nil empty map and no error", got, err)
	}
}
