package domain

import "testing"

func TestCommentStatusPolicy(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          CommentStatus
		valid, editable bool
	}{
		{"published", CommentStatusPublished, true, true},
		{"hidden", CommentStatusHidden, true, false},
		{"deleted", CommentStatusDeleted, true, false},
		{"negative", -1, false, false},
		{"unknown", 3, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.Valid(); got != tc.valid {
				t.Fatalf("Valid=%t, want %t", got, tc.valid)
			}
			if got := (Comment{Status: tc.status}).CanEditContent(); got != tc.editable {
				t.Fatalf("CanEditContent=%t, want %t", got, tc.editable)
			}
		})
	}
}
