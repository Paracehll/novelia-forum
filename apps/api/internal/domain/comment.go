package domain

import (
	"fmt"
	"strconv"
	"time"
)

type CommentSubjectType int16

const (
	CommentSubjectPost CommentSubjectType = iota
	CommentSubjectNovel
)

type CommentStatus int16

const (
	CommentStatusPublished CommentStatus = iota
	CommentStatusHidden
	CommentStatusDeleted
)

type Comment struct {
	ID             int64
	SubjectType    CommentSubjectType
	SubjectKey     string
	RootID         *int64
	Content        string
	AuthorID       int64
	AuthorUsername string
	Status         CommentStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CommentThreadPreview struct {
	Root       Comment
	ReplyCount int64
	Replies    []Comment
}

func PostCommentSubjectKey(postID int64) string {
	return strconv.FormatInt(postID, 10)
}

func PostIDFromCommentSubjectKey(subjectKey string) (int64, error) {
	postID, err := strconv.ParseInt(subjectKey, 10, 64)
	if err != nil || postID <= 0 {
		return 0, fmt.Errorf("invalid post comment subject key %q", subjectKey)
	}
	return postID, nil
}
