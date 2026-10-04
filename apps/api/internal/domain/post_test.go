package domain

import (
	"testing"
	"time"
)

func TestPostPolicies(t *testing.T) {
	for _, status := range []PostStatus{PostStatusPublished, PostStatusHidden, PostStatusDeleted, -1, 3} {
		want := status >= PostStatusPublished && status <= PostStatusDeleted
		if status.Valid() != want {
			t.Fatalf("status %d: valid=%v", status, status.Valid())
		}
	}
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	post := Post{AuthorID: 7, CreatedAt: createdAt}
	if !post.IsOwnedBy(7) || post.IsOwnedBy(8) {
		t.Fatal("unexpected ownership policy")
	}
	deadline := createdAt.Add(20 * time.Minute)
	if !post.WithinDeletionWindow(deadline) || post.WithinDeletionWindow(deadline.Add(time.Nanosecond)) {
		t.Fatal("unexpected deletion window boundary")
	}
}
