package domain

import "time"

// Tag describes a category's label independently of its storage representation.
type Tag struct {
	ID         int64
	CategoryID int64
	Name       string
	Color      int16
	IsActive   bool
	SortOrder  int32
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Attr       string
}
