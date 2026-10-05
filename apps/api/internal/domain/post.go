package domain

import "time"

// Post sort values are shared by query validation and storage ordering.
const (
	PostSortActive   = "active"
	PostSortNewest   = "newest"
	PostSortViews    = "views"
	PostSortComments = "comments"
)

type PostStatus int16

const (
	PostStatusPublished PostStatus = iota
	PostStatusHidden
	PostStatusDeleted
)

func (s PostStatus) Valid() bool {
	return s == PostStatusPublished || s == PostStatusHidden || s == PostStatusDeleted
}

// PostTag is the tag information associated with a post, not its storage model.
type PostTag struct {
	ID    int64
	Name  string
	Color int16
}

// PostTags projects category tags to the metadata exposed on posts.
// It preserves display order and returns an empty slice for empty input.
func PostTags(tags []Tag) []PostTag {
	result := make([]PostTag, len(tags))
	for i, tag := range tags {
		result[i] = PostTag{ID: tag.ID, Name: tag.Name, Color: tag.Color}
	}
	return result
}

type Post struct {
	ID             int64
	CategoryID     int64
	Title          string
	AuthorID       int64
	AuthorUsername string
	Content        string
	Status         PostStatus
	ViewsCount     int32
	CommentsCount  int32
	CommentsLocked bool
	PinOrder       *int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ActiveAt       time.Time
	Tags           []PostTag
}

func (p Post) IsOwnedBy(userID int64) bool {
	return p.AuthorID == userID
}

// WithinDeletionWindow defines the author deletion policy. Admin permissions
// are evaluated separately by the usecase.
func (p Post) WithinDeletionWindow(now time.Time) bool {
	return !now.After(p.CreatedAt.Add(20 * time.Minute))
}

// PostListItem contains only the metadata needed by post lists. It deliberately
// does not embed Post, so a list can never expose the post content.
type PostListItem struct {
	ID             int64
	CategoryID     int64
	Title          string
	AuthorID       int64
	AuthorUsername string
	Status         PostStatus
	ViewsCount     int32
	CommentsCount  int32
	CommentsLocked bool
	PinOrder       *int32
	Favorited      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ActiveAt       time.Time
	Tags           []PostTag
}
