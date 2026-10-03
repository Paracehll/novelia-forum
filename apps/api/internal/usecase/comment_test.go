package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"auth/internal/repository"
	"auth/internal/subject"

	"github.com/go-jet/jet/v2/qrm"
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
	return 1, []repository.CommentThread{{Comment: r.comment}}, nil
}
func (r *commentRepoStub) ListReplies(int16, string, int64, int64, int64) (int64, []repository.Comment, error) {
	return 1, []repository.Comment{r.comment}, nil
}

type subjectCheckFunc func(context.Context, string, string) (int16, error)

func (f subjectCheckFunc) Check(ctx context.Context, kind, key string) (int16, error) {
	return f(ctx, kind, key)
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
				checker := subjectCheckFunc(func(context.Context, string, string) (int16, error) {
					checks++
					return repository.CommentSubjectNovel, nil
				})
				u := NewCommentUsecase(repo, nil, nil, checker)
				input := repository.CreateCommentInput{SubjectKey: "42", Content: tc.content, AuthorID: 1}
				var err error
				switch operation {
				case "create post":
					_, err = u.CreatePost(input)
				case "create external":
					_, err = u.CreateExternal(context.Background(), "novel", input)
				default:
					subjectType := repository.CommentSubjectPost
					if operation == "update external" {
						subjectType = repository.CommentSubjectNovel
					}
					_, err = u.Update(Actor{UserID: 1}, subjectType, 7, tc.content)
				}
				if !tc.valid {
					if !errors.Is(err, ErrCommentContentInvalid) || repo.writes != 0 || checks != 0 {
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
	for _, subjectType := range []int16{repository.CommentSubjectPost, repository.CommentSubjectNovel} {
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
				t.Run(operation+"/"+tc.name+"/"+repository.PostSubjectKey(int64(subjectType)), func(t *testing.T) {
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
						_, err = u.Update(tc.principal, subjectType, 7, "updated")
					} else {
						err = u.Delete(tc.principal, subjectType, 7)
					}
					if tc.allowed {
						if err != nil || repo.writes != 1 {
							t.Fatalf("err=%v writes=%d", err, repo.writes)
						}
					} else {
						wantErr := ErrCommentNotOwner
						if tc.name == "expired" {
							wantErr = ErrCommentEditExpired
						}
						if !errors.Is(err, wantErr) || repo.writes != 0 {
							t.Fatalf("err=%v writes=%d", err, repo.writes)
						}
					}
				})
			}
		}
	}
}

func TestExternalCommentChecksBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{"exists", nil, nil},
		{"missing", subject.ErrNotFound, ErrCommentSubjectNotFound},
		{"invalid", subject.ErrInvalid, subject.ErrInvalid},
		{"unsupported", subject.ErrUnsupported, subject.ErrUnsupported},
		{"unavailable", errors.New("upstream unavailable"), ErrCommentSubjectUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &commentRepoStub{}
			checks := 0
			checker := subjectCheckFunc(func(ctx context.Context, kind, key string) (int16, error) {
				checks++
				if kind != "novel" || key != "web-syosetu-n1234" {
					t.Fatalf("unexpected subject: %s/%s", kind, key)
				}
				return repository.CommentSubjectNovel, tc.err
			})
			u := NewCommentUsecase(repo, nil, nil, checker)
			input := repository.CreateCommentInput{
				SubjectKey: "web-syosetu-n1234",
				Content:    "body",
				AuthorID:   1,
			}
			_, err := u.CreateExternal(context.Background(), "novel", input)
			if checks != 1 {
				t.Fatalf("checks=%d", checks)
			}
			if tc.wantErr == nil {
				if err != nil || repo.writes != 1 ||
					repo.input.SubjectType != repository.CommentSubjectNovel ||
					repo.input.Attr != "{}" || repo.input.AuthorID != 1 {
					t.Fatalf("err=%v writes=%d input=%+v", err, repo.writes, repo.input)
				}
			} else {
				if !errors.Is(err, tc.wantErr) || repo.writes != 0 {
					t.Fatalf("err=%v writes=%d", err, repo.writes)
				}
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
	checker := subjectCheckFunc(func(context.Context, string, string) (int16, error) {
		return repository.CommentSubjectNovel, nil
	})
	for _, tc := range []struct {
		name    string
		call    func(*CommentUsecase) error
		missing error
	}{
		{"comment.create_post", func(u *CommentUsecase) error {
			_, err := u.CreatePost(repository.CreateCommentInput{SubjectKey: "42", Content: "body"})
			return err
		}, ErrCommentSubjectNotFound},
		{"comment.create_external", func(u *CommentUsecase) error {
			_, err := u.CreateExternal(context.Background(), "novel", repository.CreateCommentInput{SubjectKey: "wenku-book", Content: "body"})
			return err
		}, ErrCommentRootNotFound},
		{"comment.list_replies", func(u *CommentUsecase) error {
			_, _, err := u.ListReplies(repository.CommentSubjectNovel, "wenku-book", 7, 20, 0)
			return err
		}, ErrCommentRootNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("private database details")
			u := NewCommentUsecase(failingCommentRepository{err: cause}, nil, nil, checker)
			err := tc.call(u)
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("expected operation and original cause, got %v", err)
			}
			u = NewCommentUsecase(failingCommentRepository{err: fmt.Errorf("query: %w", qrm.ErrNoRows)}, nil, nil, checker)
			if err := tc.call(u); !errors.Is(err, tc.missing) {
				t.Fatalf("expected %v, got %v", tc.missing, err)
			}
		})
	}
}

type missingPostRepository struct{ repository.PostRepository }

func (missingPostRepository) Find(int64, bool) (*repository.PostDetails, error) {
	return nil, qrm.ErrNoRows
}

func TestCommentReadSubjectChecks(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) (int16, error) {
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
			_, _, err = missing.ListReplies(repository.CommentSubjectPost, "42", 7, 20, 0)
		} else {
			_, _, err = missing.ListRoots(repository.CommentSubjectPost, "42", 20, 0)
		}
		if !errors.Is(err, ErrCommentSubjectNotFound) {
			t.Fatalf("expected missing post, got %v", err)
		}
		if replies {
			_, _, err = external.ListReplies(repository.CommentSubjectNovel, "deleted", 7, 20, 0)
		} else {
			_, _, err = external.ListRoots(repository.CommentSubjectNovel, "deleted", 20, 0)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
