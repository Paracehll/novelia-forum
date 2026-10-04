package usecase

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"forum/internal/domain"
	"forum/internal/repository"
)

func TestCommentEditingStatePolicy(t *testing.T) {
	for _, external := range []bool{false, true} {
		for _, actor := range []Actor{{UserID: 1}, {UserID: 2, IsAdmin: true}, {UserID: 3}} {
			for _, status := range []domain.CommentStatus{domain.CommentStatusPublished, domain.CommentStatusHidden, domain.CommentStatusDeleted, -1, 99} {
				t.Run(fmt.Sprintf("external=%t/user=%d/admin=%t/status=%d", external, actor.UserID, actor.IsAdmin, status), func(t *testing.T) {
					repo := &commentRepoStub{comment: domain.Comment{ID: 7, AuthorID: 1, Content: "original", Status: status, CreatedAt: time.Now()}}
					u := NewCommentUsecase(repo, nil, nil, subjectResolverStub{supported: true})
					var comment *domain.Comment
					var err error
					if external {
						comment, err = u.UpdateExternal(actor, UpdateExternalCommentCommand{Kind: "novel", CommentID: 7, Content: "updated"})
					} else {
						comment, err = u.Update(actor, UpdateCommentCommand{SubjectType: domain.CommentSubjectPost, CommentID: 7, Content: "updated"})
					}
					if !actor.IsAdmin && actor.UserID != 1 {
						if !isAppErrorCode(err, CodeCommentNotOwner) || comment != nil || repo.writes != 0 {
							t.Fatalf("authorization must precede state checks: comment=%v err=%v writes=%d", comment, err, repo.writes)
						}
						return
					}
					if status == domain.CommentStatusPublished {
						if err != nil || comment == nil || comment.Content != "updated" || repo.writes != 1 {
							t.Fatalf("comment=%v err=%v writes=%d", comment, err, repo.writes)
						}
						return
					}
					var appErr *AppError
					if !errors.As(err, &appErr) || appErr.Kind != KindConflict || appErr.Code != CodeCommentNotEditable || comment != nil || repo.writes != 0 || repo.comment.Content != "original" {
						t.Fatalf("comment=%v err=%v writes=%d content=%q", comment, err, repo.writes, repo.comment.Content)
					}
				})
			}
		}
	}
}

func TestCommentDeletingNonPublishedStateRemainsAllowed(t *testing.T) {
	for _, subjectType := range []domain.CommentSubjectType{domain.CommentSubjectPost, domain.CommentSubjectNovel} {
		for _, actor := range []Actor{{UserID: 1}, {UserID: 2, IsAdmin: true}} {
			for _, status := range []domain.CommentStatus{domain.CommentStatusHidden, domain.CommentStatusDeleted} {
				t.Run(fmt.Sprintf("type=%d/admin=%t/status=%d", subjectType, actor.IsAdmin, status), func(t *testing.T) {
					repo := &commentRepoStub{comment: domain.Comment{ID: 7, AuthorID: 1, Status: status, CreatedAt: time.Now()}}
					u := NewCommentUsecase(repo, nil, nil, nil)
					if err := u.Delete(actor, DeleteCommentCommand{SubjectType: subjectType, CommentID: 7}); err != nil || repo.writes != 1 || repo.comment.Status != domain.CommentStatusDeleted {
						t.Fatalf("err=%v writes=%d status=%d", err, repo.writes, repo.comment.Status)
					}
				})
			}
		}
	}
}

type changingCommentRepoStub struct {
	commentRepoStub
	findErr, updateErr error
}

func (r *changingCommentRepoStub) Find(domain.CommentSubjectType, int64) (*domain.Comment, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return &r.comment, nil
}

func (r *changingCommentRepoStub) Update(domain.CommentSubjectType, int64, string) (*domain.Comment, error) {
	r.writes++
	return nil, r.updateErr
}

func TestCommentUpdateStorageFailures(t *testing.T) {
	cause := errors.New("private storage failure")
	for _, tc := range []struct {
		name               string
		findErr, updateErr error
		wantKind           ErrorKind
		wantCode           string
		wantWrites         int
	}{
		{"missing at lookup", fmt.Errorf("find: %w", repository.ErrNotFound), nil, KindNotFound, CodeCommentNotFound, 0},
		{"conditional update rejected", nil, fmt.Errorf("update: %w", repository.ErrNotFound), KindConflict, CodeCommentConflict, 1},
		{"storage failure", nil, cause, "", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &changingCommentRepoStub{commentRepoStub: commentRepoStub{comment: domain.Comment{AuthorID: 1, CreatedAt: time.Now()}}, findErr: tc.findErr, updateErr: tc.updateErr}
			u := NewCommentUsecase(repo, nil, nil, nil)
			comment, err := u.Update(Actor{UserID: 1}, UpdateCommentCommand{CommentID: 7, Content: "updated"})
			if comment != nil || err == nil || repo.writes != tc.wantWrites {
				t.Fatalf("comment=%v err=%v writes=%d", comment, err, repo.writes)
			}
			if tc.wantCode == "" {
				if !errors.Is(err, cause) {
					t.Fatalf("storage cause lost: %v", err)
				}
				return
			}
			var appErr *AppError
			if !errors.As(err, &appErr) || appErr.Kind != tc.wantKind || appErr.Code != tc.wantCode {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCommentAdminStatusValidation(t *testing.T) {
	for _, status := range []domain.CommentStatus{domain.CommentStatusPublished, domain.CommentStatusHidden, domain.CommentStatusDeleted, -1, 3, 32767} {
		for _, operation := range []struct {
			name string
			call func(*CommentUsecase, Actor) error
		}{
			{"set status", func(u *CommentUsecase, actor Actor) error {
				return u.SetStatus(actor, SetCommentStatusCommand{CommentID: 7, Status: status})
			}},
			{"set external status", func(u *CommentUsecase, actor Actor) error {
				return u.SetExternalStatus(actor, SetExternalCommentStatusCommand{Kind: "novel", CommentID: 7, Status: status})
			}},
			{"list", func(u *CommentUsecase, actor Actor) error {
				_, _, err := u.ListAdmin(actor, ListAdminCommentsQuery{Status: &status})
				return err
			}},
		} {
			for _, admin := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/status=%d/admin=%t", operation.name, status, admin), func(t *testing.T) {
					repo := &adminCommentRepoStub{}
					u := NewCommentUsecase(repo, nil, nil, subjectResolverStub{supported: true})
					err := operation.call(u, Actor{UserID: 1, IsAdmin: admin})
					if admin && status.Valid() {
						if err != nil || repo.calls != 1 {
							t.Fatalf("err=%v calls=%d", err, repo.calls)
						}
						return
					}
					wantKind, wantCode := KindPermissionDenied, CodeCommentAdminRequired
					if admin {
						wantKind, wantCode = KindInvalid, CodeCommentStatusInvalid
					}
					var appErr *AppError
					if !errors.As(err, &appErr) || appErr.Kind != wantKind || appErr.Code != wantCode || repo.calls != 0 {
						t.Fatalf("err=%v calls=%d", err, repo.calls)
					}
				})
			}
		}
	}
}
