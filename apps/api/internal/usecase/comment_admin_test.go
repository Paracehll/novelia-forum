package usecase

import (
	"errors"
	"reflect"
	"testing"

	"forum/internal/domain"
	"forum/internal/repository"
)

type adminCommentRepoStub struct {
	repository.CommentRepository
	calls         int
	err           error
	filter        repository.CommentFilter
	limit, offset int64
	subjectType   domain.CommentSubjectType
	commentID     int64
	status        domain.CommentStatus
	authorID      int64
}

func (r *adminCommentRepoStub) ListAdmin(filter repository.CommentFilter, limit, offset int64) (int64, []domain.Comment, error) {
	r.calls++
	r.filter, r.limit, r.offset = filter, limit, offset
	return 1, []domain.Comment{{ID: 7, Content: "hidden body", Status: domain.CommentStatusHidden}}, r.err
}

func (r *adminCommentRepoStub) SetStatus(subjectType domain.CommentSubjectType, id int64, status domain.CommentStatus) error {
	r.calls++
	r.subjectType, r.commentID, r.status = subjectType, id, status
	return r.err
}

func (r *adminCommentRepoStub) DeleteAllByAuthor(authorID int64) error {
	r.calls++
	r.authorID = authorID
	return r.err
}

func TestCommentAdminPermissions(t *testing.T) {
	for _, operation := range []struct {
		name string
		call func(*CommentUsecase, Actor) error
	}{
		{"list", func(u *CommentUsecase, actor Actor) error {
			_, _, err := u.ListAdmin(actor, ListAdminCommentsQuery{})
			return err
		}},
		{"set post status", func(u *CommentUsecase, actor Actor) error {
			return u.SetStatus(actor, SetCommentStatusCommand{SubjectType: domain.CommentSubjectPost, CommentID: 7, Status: domain.CommentStatusHidden})
		}},
		{"set external status", func(u *CommentUsecase, actor Actor) error {
			return u.SetExternalStatus(actor, SetExternalCommentStatusCommand{Kind: "novel", CommentID: 7, Status: domain.CommentStatusHidden})
		}},
		{"delete by author", func(u *CommentUsecase, actor Actor) error {
			return u.DeleteAllByAuthor(actor, DeleteCommentsByAuthorCommand{AuthorID: 1})
		}},
	} {
		for _, tc := range []struct {
			name  string
			actor Actor
		}{
			{"anonymous", Actor{}},
			{"author", Actor{UserID: 1}},
			{"other member", Actor{UserID: 2}},
			{"admin", Actor{UserID: 2, IsAdmin: true}},
		} {
			t.Run(operation.name+"/"+tc.name, func(t *testing.T) {
				repo := &adminCommentRepoStub{}
				u := NewCommentUsecase(repo, nil, nil, subjectResolverStub{supported: true})
				err := operation.call(u, tc.actor)
				if tc.actor.IsAdmin {
					if err != nil || repo.calls != 1 {
						t.Fatalf("err=%v calls=%d", err, repo.calls)
					}
					return
				}
				var appErr *AppError
				if !errors.As(err, &appErr) || appErr.Kind != KindForbidden || appErr.Code != CodeCommentAdminRequired || repo.calls != 0 {
					t.Fatalf("err=%v calls=%d", err, repo.calls)
				}
			})
		}
	}
}

func TestCommentExternalStatusChecksPermissionBeforeSubject(t *testing.T) {
	// Missing resolver/repository ensure authorization is independent of subject support.
	u := NewCommentUsecase(nil, nil, nil, nil)
	command := SetExternalCommentStatusCommand{Kind: "unknown", CommentID: 7}
	if err := u.SetExternalStatus(Actor{UserID: 1}, command); !isAppErrorCode(err, CodeCommentAdminRequired) {
		t.Fatalf("expected authorization failure, got %v", err)
	}
	if err := u.SetExternalStatus(Actor{IsAdmin: true}, command); !isAppErrorCode(err, CodeCommentSubjectTypeInvalid) {
		t.Fatalf("expected unsupported subject, got %v", err)
	}
}

func TestCommentAdminCommands(t *testing.T) {
	repo := &adminCommentRepoStub{}
	u := NewCommentUsecase(repo, nil, nil, subjectResolverStub{supported: true})
	actor := Actor{UserID: 2, IsAdmin: true}
	status := domain.CommentStatusHidden
	query := ListAdminCommentsQuery{Search: "body", AuthorName: "alice", PostID: 42, Status: &status, Limit: 10, Offset: 20}
	total, items, err := u.ListAdmin(actor, query)
	wantFilter := repository.CommentFilter{Search: query.Search, AuthorName: query.AuthorName, PostID: query.PostID, Status: query.Status}
	if err != nil || total != 1 || len(items) != 1 || items[0].Content != "hidden body" || !reflect.DeepEqual(repo.filter, wantFilter) || repo.limit != query.Limit || repo.offset != query.Offset {
		t.Fatalf("total=%d items=%v err=%v repo=%+v", total, items, err, repo)
	}
	if err := u.SetStatus(actor, SetCommentStatusCommand{SubjectType: domain.CommentSubjectPost, CommentID: 7, Status: status}); err != nil || repo.subjectType != domain.CommentSubjectPost || repo.commentID != 7 || repo.status != status {
		t.Fatalf("err=%v repo=%+v", err, repo)
	}
	if err := u.SetExternalStatus(actor, SetExternalCommentStatusCommand{Kind: "novel", CommentID: 8, Status: domain.CommentStatusPublished}); err != nil || repo.subjectType != domain.CommentSubjectNovel || repo.commentID != 8 || repo.status != domain.CommentStatusPublished {
		t.Fatalf("err=%v repo=%+v", err, repo)
	}
	if err := u.DeleteAllByAuthor(actor, DeleteCommentsByAuthorCommand{AuthorID: 42}); err != nil || repo.authorID != 42 {
		t.Fatalf("err=%v repo=%+v", err, repo)
	}
}
