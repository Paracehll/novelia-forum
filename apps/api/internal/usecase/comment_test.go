package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"forum/internal/domain"
	"forum/internal/repository"
	"forum/internal/subject"
)

type commentRepoStub struct {
	repository.CommentRepository
	comment repository.Comment
	writes  int
	input   repository.CreateCommentInput
}

func (r *commentRepoStub) Find(int16, int64) (*repository.Comment, error) { return &r.comment, nil }
func (r *commentRepoStub) Update(_ int16, _ int64, content string) (*repository.Comment, error) {
	r.writes++
	r.comment.Content = content
	return &r.comment, nil
}
func (r *commentRepoStub) SetStatus(_ int16, _ int64, status int16) error {
	r.writes++
	r.comment.Status = status
	return nil
}
func (r *commentRepoStub) Create(input repository.CreateCommentInput) (*repository.Comment, error) {
	r.writes++
	r.input = input
	return &r.comment, nil
}
func (r *commentRepoStub) ListRoots(int16, string, int64, int64) (int64, []repository.CommentThread, error) {
	return 1, []repository.CommentThread{{Comment: r.comment, ReplyCount: 1, Replies: []repository.Comment{r.comment}}}, nil
}
func (r *commentRepoStub) ListReplies(int16, string, int64, int64, int64) (int64, []repository.Comment, error) {
	return 1, []repository.Comment{r.comment}, nil
}

type subjectCheckFunc func(context.Context, string, string) (domain.CommentSubjectType, error)

func (f subjectCheckFunc) Check(ctx context.Context, kind, key string) (domain.CommentSubjectType, error) {
	return f(ctx, kind, key)
}

func isAppErrorCode(err error, code string) bool {
	var appErr *AppError
	return errors.As(err, &appErr) && appErr.Code == code
}

func TestCommentContentValidation(t *testing.T) {
	for _, operation := range []string{"create post", "create external", "update post", "update external"} {
		for _, tc := range []struct {
			name    string
			content string
			valid   bool
		}{
			{"empty", "", false},
			{"whitespace", " \n\t\u3000", false},
			{"too long", strings.Repeat("字", 1001), false},
			{"padding counts", strings.Repeat("字", 1000) + " ", false},
			{"max length unicode", strings.Repeat("😀", 1000), true},
			{"preserve whitespace", " 评论 \n", true},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				repo := &commentRepoStub{comment: repository.Comment{AuthorID: 1, CreatedAt: time.Now()}}
				checks := 0
				checker := subjectCheckFunc(func(context.Context, string, string) (domain.CommentSubjectType, error) {
					checks++
					return domain.CommentSubjectNovel, nil
				})
				u := NewCommentUsecase(repo, nil, nil, checker)
				actor := Actor{UserID: 1, Username: "tester"}
				var err error
				switch operation {
				case "create post":
					_, err = u.Create(actor, CreatePostCommentCommand{PostID: 42, Content: tc.content})
				case "create external":
					_, err = u.CreateExternal(context.Background(), actor, CreateExternalCommentCommand{
						Kind: "novel", SubjectKey: "42", Content: tc.content,
					})
				default:
					subjectType := domain.CommentSubjectPost
					if operation == "update external" {
						subjectType = domain.CommentSubjectNovel
					}
					_, err = u.Update(actor, UpdateCommentCommand{
						SubjectType: subjectType, CommentID: 7, Content: tc.content,
					})
				}
				if !tc.valid {
					if !isAppErrorCode(err, CodeCommentContentInvalid) || repo.writes != 0 || checks != 0 {
						t.Fatalf("err=%v writes=%d checks=%d", err, repo.writes, checks)
					}
					return
				}
				if err != nil || repo.writes != 1 {
					t.Fatalf("err=%v writes=%d", err, repo.writes)
				}
				written := repo.input.Content
				if strings.HasPrefix(operation, "update") {
					written = repo.comment.Content
				}
				if written != tc.content {
					t.Fatalf("content changed: %q", written)
				}
			})
		}
	}
}

func TestCommentModificationPermissions(t *testing.T) {
	for _, subjectType := range []domain.CommentSubjectType{domain.CommentSubjectPost, domain.CommentSubjectNovel} {
		for _, operation := range []string{"update", "delete"} {
			for _, tc := range []struct {
				name      string
				principal Actor
				age       time.Duration
				allowed   bool
			}{
				{"author", Actor{UserID: 1}, 19 * time.Minute, true},
				{"expired", Actor{UserID: 1}, 21 * time.Minute, false},
				{"other author", Actor{UserID: 2}, time.Minute, false},
				{"admin", Actor{UserID: 2, IsAdmin: true}, time.Hour, true},
			} {
				t.Run(operation+"/"+tc.name+"/"+domain.PostCommentSubjectKey(int64(subjectType)), func(t *testing.T) {
					repo := &commentRepoStub{
						comment: repository.Comment{
							ID:        7,
							AuthorID:  1,
							CreatedAt: time.Now().Add(-tc.age),
						},
					}
					u := NewCommentUsecase(repo, nil, nil, nil)
					var err error
					if operation == "update" {
						_, err = u.Update(tc.principal, UpdateCommentCommand{
							SubjectType: subjectType, CommentID: 7, Content: "updated",
						})
					} else {
						err = u.Delete(tc.principal, DeleteCommentCommand{SubjectType: subjectType, CommentID: 7})
					}
					if tc.allowed {
						if err != nil || repo.writes != 1 {
							t.Fatalf("err=%v writes=%d", err, repo.writes)
						}
					} else {
						wantCode := CodeCommentNotOwner
						if tc.name == "expired" {
							wantCode = CodeCommentEditExpired
						}
						if !isAppErrorCode(err, wantCode) || repo.writes != 0 {
							t.Fatalf("err=%v writes=%d", err, repo.writes)
						}
					}
				})
			}
		}
	}
}

func TestPostCommentCreationUsesActor(t *testing.T) {
	repo := &commentRepoStub{}
	u := NewCommentUsecase(repo, nil, nil, nil)
	actor := Actor{UserID: 42, Username: "alice"}

	if _, err := u.Create(actor, CreatePostCommentCommand{PostID: 7, Content: "body"}); err != nil {
		t.Fatal(err)
	}
	if repo.input.AuthorID != actor.UserID || repo.input.AuthorUsername != actor.Username {
		t.Fatalf("persisted author = %d/%q, want %d/%q",
			repo.input.AuthorID, repo.input.AuthorUsername, actor.UserID, actor.Username)
	}
}

func TestExternalCommentChecksBeforeWrite(t *testing.T) {
	upstreamErr := errors.New("upstream unavailable")
	for _, tc := range []struct {
		name      string
		err       error
		wantCode  string
		wantCause error
	}{
		{"exists", nil, "", nil},
		{"missing", subject.ErrNotFound, CodeCommentSubjectNotFound, nil},
		{"invalid", subject.ErrInvalid, CodeCommentSubjectKeyInvalid, nil},
		{"unsupported", subject.ErrUnsupported, CodeCommentSubjectTypeInvalid, nil},
		{"unavailable", upstreamErr, "", upstreamErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &commentRepoStub{}
			checks := 0
			checker := subjectCheckFunc(func(ctx context.Context, kind, key string) (domain.CommentSubjectType, error) {
				checks++
				if kind != "novel" || key != "web-syosetu-n1234" {
					t.Fatalf("unexpected subject: %s/%s", kind, key)
				}
				return domain.CommentSubjectNovel, tc.err
			})
			u := NewCommentUsecase(repo, nil, nil, checker)
			actor := Actor{UserID: 1, Username: "tester"}
			command := CreateExternalCommentCommand{
				Kind: "novel", SubjectKey: "web-syosetu-n1234", Content: "body",
			}
			_, err := u.CreateExternal(context.Background(), actor, command)
			if checks != 1 {
				t.Fatalf("checks=%d", checks)
			}
			if tc.wantCode == "" && tc.wantCause == nil {
				if err != nil || repo.writes != 1 ||
					repo.input.SubjectType != repository.CommentSubjectNovel ||
					repo.input.Attr != "{}" || repo.input.AuthorID != actor.UserID ||
					repo.input.AuthorUsername != actor.Username {
					t.Fatalf("err=%v writes=%d input=%+v", err, repo.writes, repo.input)
				}
				return
			}
			if repo.writes != 0 {
				t.Fatalf("unexpected writes: %d", repo.writes)
			}
			if tc.wantCode != "" && !isAppErrorCode(err, tc.wantCode) {
				t.Fatalf("got %v, want code %s", err, tc.wantCode)
			}
			if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
				t.Fatalf("got %v, want cause %v", err, tc.wantCause)
			}
		})
	}
}

type failingCommentRepository struct {
	repository.CommentRepository
	err error
}

func (r failingCommentRepository) Create(repository.CreateCommentInput) (*repository.Comment, error) {
	return nil, r.err
}

func (r failingCommentRepository) ListReplies(int16, string, int64, int64, int64) (int64, []repository.Comment, error) {
	return 0, nil, r.err
}

func TestCommentRepositoryErrors(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) (domain.CommentSubjectType, error) {
		return domain.CommentSubjectNovel, nil
	})
	for _, tc := range []struct {
		name         string
		call         func(*CommentUsecase) error
		missingCode  string
		storageError error
	}{
		{"comment.create_post", func(u *CommentUsecase) error {
			_, err := u.Create(Actor{}, CreatePostCommentCommand{PostID: 42, Content: "body"})
			return err
		}, CodeCommentSubjectNotFound, repository.ErrNotFound},
		{"comment.create_external", func(u *CommentUsecase) error {
			_, err := u.CreateExternal(context.Background(), Actor{}, CreateExternalCommentCommand{
				Kind: "novel", SubjectKey: "wenku-book", Content: "body",
			})
			return err
		}, CodeCommentRootNotFound, repository.ErrCommentRootNotFound},
		{"comment.list_replies", func(u *CommentUsecase) error {
			_, _, err := u.ListReplies(Actor{}, ListCommentRepliesQuery{
				SubjectType: domain.CommentSubjectNovel, SubjectKey: "wenku-book", RootID: 7, Limit: 20,
			})
			return err
		}, CodeCommentRootNotFound, repository.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("private database details")
			u := NewCommentUsecase(failingCommentRepository{err: cause}, nil, nil, checker)
			err := tc.call(u)
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("expected operation and original cause, got %v", err)
			}
			u = NewCommentUsecase(failingCommentRepository{err: fmt.Errorf("query: %w", tc.storageError)}, nil, nil, checker)
			if err := tc.call(u); !isAppErrorCode(err, tc.missingCode) {
				t.Fatalf("expected code %s, got %v", tc.missingCode, err)
			}
		})
	}
}

type missingPostRepository struct{ repository.PostRepository }

func (missingPostRepository) Find(int64, bool) (*repository.PostDetails, error) {
	return nil, repository.ErrNotFound
}

func TestCommentReadSubjectChecks(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) (domain.CommentSubjectType, error) {
		t.Fatal("reads must not check external resources")
		return 0, nil
	})
	// Unimplemented comment queries panic if a missing post reaches the repository.
	missing := NewCommentUsecase(
		&struct{ repository.CommentRepository }{},
		missingPostRepository{},
		nil,
		checker,
	)
	external := NewCommentUsecase(&commentRepoStub{}, nil, nil, checker)
	for _, replies := range []bool{false, true} {
		var err error
		if replies {
			_, _, err = missing.ListReplies(Actor{}, ListCommentRepliesQuery{
				SubjectType: domain.CommentSubjectPost, SubjectKey: "42", RootID: 7, Limit: 20,
			})
		} else {
			_, _, err = missing.List(Actor{}, ListCommentsQuery{
				SubjectType: domain.CommentSubjectPost, SubjectKey: "42", Limit: 20,
			})
		}
		if !isAppErrorCode(err, CodeCommentSubjectNotFound) {
			t.Fatalf("expected missing post, got %v", err)
		}
		if replies {
			_, _, err = external.ListReplies(Actor{}, ListCommentRepliesQuery{
				SubjectType: domain.CommentSubjectNovel, SubjectKey: "deleted", RootID: 7, Limit: 20,
			})
		} else {
			_, _, err = external.List(Actor{}, ListCommentsQuery{
				SubjectType: domain.CommentSubjectNovel, SubjectKey: "deleted", Limit: 20,
			})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommentCreationStorageFailures(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) (domain.CommentSubjectType, error) {
		return domain.CommentSubjectNovel, nil
	})
	for _, external := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			storage  error
			wantCode string
		}{
			{"missing root", repository.ErrCommentRootNotFound, CodeCommentRootNotFound},
			{"invalid root", repository.ErrInvalidCommentRoot, CodeCommentRootInvalid},
			{"conflict", repository.ErrConflict, CodeCommentConflict},
		} {
			t.Run(fmt.Sprintf("external=%t/%s", external, tc.name), func(t *testing.T) {
				u := NewCommentUsecase(failingCommentRepository{err: fmt.Errorf("create: %w", tc.storage)}, nil, nil, checker)
				rootID := int64(7)
				var err error
				if external {
					_, err = u.CreateExternal(context.Background(), Actor{}, CreateExternalCommentCommand{
						Kind: "novel", SubjectKey: "42", RootID: &rootID, Content: "reply",
					})
				} else {
					_, err = u.Create(Actor{}, CreatePostCommentCommand{PostID: 42, RootID: &rootID, Content: "reply"})
				}
				if !isAppErrorCode(err, tc.wantCode) {
					t.Fatalf("got %v, want code %s", err, tc.wantCode)
				}
			})
		}
	}
}

func TestCommentListContentVisibility(t *testing.T) {
	for _, actor := range []Actor{{}, {UserID: 1}, {UserID: 2, IsAdmin: true}} {
		for _, status := range []int16{repository.StatusPublished, repository.StatusHidden, repository.StatusDeleted, 99} {
			t.Run(fmt.Sprintf("actor=%+v/status=%d", actor, status), func(t *testing.T) {
				repo := &commentRepoStub{comment: repository.Comment{ID: 7, AuthorID: 1, Content: "body", Status: status}}
				u := NewCommentUsecase(repo, nil, nil, nil)
				want := repo.comment
				if !actor.IsAdmin && status != repository.StatusPublished {
					want.Content = ""
				}
				total, roots, err := u.List(actor, ListCommentsQuery{
					SubjectType: domain.CommentSubjectNovel, SubjectKey: "book", Limit: 20,
				})
				if err != nil || total != 1 || len(roots) != 1 {
					t.Fatalf("roots=%+v total=%d err=%v", roots, total, err)
				}
				wantDomain := commentFromRepository(want)
				if roots[0].Root != wantDomain || roots[0].ReplyCount != 1 || len(roots[0].Replies) != 1 || roots[0].Replies[0] != wantDomain {
					t.Fatalf("unexpected thread: %+v", roots[0])
				}
				total, replies, err := u.ListReplies(actor, ListCommentRepliesQuery{
					SubjectType: domain.CommentSubjectNovel, SubjectKey: "book", RootID: 7, Limit: 20,
				})
				if err != nil || total != 1 || len(replies) != 1 || replies[0] != wantDomain {
					t.Fatalf("replies=%+v total=%d err=%v", replies, total, err)
				}
			})
		}
	}
}
