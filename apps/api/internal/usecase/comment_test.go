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
)

type commentRepoStub struct {
	repository.CommentRepository
	comment domain.Comment
	writes  int
	input   domain.Comment
}

func (r *commentRepoStub) Find(context.Context, domain.CommentSubjectType, int64) (*domain.Comment, error) {
	return &r.comment, nil
}
func (r *commentRepoStub) Lock(_ context.Context, kind domain.CommentSubjectType, _ int64) (*domain.Comment, error) {
	copy := r.comment
	copy.SubjectType = kind
	if copy.SubjectKey == "" {
		copy.SubjectKey = "42"
	}
	return &copy, nil
}
func (r *commentRepoStub) Update(
	ctx context.Context,
	_ domain.CommentSubjectType,
	_ int64,
	content string,
) (*domain.Comment, error) {
	r.writes++
	r.comment.Content = content
	return &r.comment, nil
}
func (r *commentRepoStub) SetStatus(
	ctx context.Context,
	_ domain.CommentSubjectType,
	_ int64,
	status domain.CommentStatus,
) error {
	r.writes++
	r.comment.Status = status
	return nil
}
func (r *commentRepoStub) Create(ctx context.Context, input domain.Comment) (*domain.Comment, error) {
	r.writes++
	r.input = input
	return &r.comment, nil
}
func (r *commentRepoStub) ListRoots(
	context.Context,
	domain.CommentSubjectType,
	string,
	int64,
	int64,
) (int64, []domain.CommentThreadPreview, error) {
	return 1, []domain.CommentThreadPreview{{Root: r.comment, ReplyCount: 1, Replies: []domain.Comment{r.comment}}}, nil
}
func (r *commentRepoStub) ListReplies(
	context.Context,
	domain.CommentSubjectType,
	string,
	int64,
	int64,
	int64,
) (int64, []domain.Comment, error) {
	return 1, []domain.Comment{r.comment}, nil
}

type subjectCheckFunc func(context.Context, string, string) bool

func (f subjectCheckFunc) Type(kind string) (domain.CommentSubjectType, bool) {
	return domain.CommentSubjectNovel, kind == "novel"
}
func (f subjectCheckFunc) Valid(kind, _ string) bool { return kind == "novel" }
func (f subjectCheckFunc) Check(ctx context.Context, kind, key string) (bool, error) {
	return f(ctx, kind, key), nil
}

type subjectResolverStub struct {
	supported bool
	valid     bool
	exists    bool
	checkErr  error
	checks    *int
}

func (s subjectResolverStub) Type(string) (domain.CommentSubjectType, bool) {
	return domain.CommentSubjectNovel, s.supported
}
func (s subjectResolverStub) Valid(string, string) bool { return s.valid }
func (s subjectResolverStub) Check(context.Context, string, string) (bool, error) {
	if s.checks != nil {
		*s.checks++
	}
	return s.exists, s.checkErr
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
				repo := &commentRepoStub{comment: domain.Comment{AuthorID: 1, CreatedAt: time.Now()}}
				checks := 0
				checker := subjectCheckFunc(func(context.Context, string, string) bool {
					checks++
					return true
				})
				u := NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, checker)
				actor := Actor{UserID: 1, Username: "tester", CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}
				var err error
				switch operation {
				case "create post":
					_, err = u.Create(context.Background(), actor, CreatePostCommentCommand{PostID: 42, Content: tc.content})
				case "create external":
					_, err = u.CreateExternal(context.Background(), actor, CreateExternalCommentCommand{
						Kind: "novel", SubjectKey: "42", Content: tc.content,
					})
				default:
					subjectType := domain.CommentSubjectPost
					if operation == "update external" {
						subjectType = domain.CommentSubjectNovel
					}
					_, err = u.Update(context.Background(), actor, UpdateCommentCommand{
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
						comment: domain.Comment{
							ID:        7,
							AuthorID:  1,
							CreatedAt: time.Now().Add(-tc.age),
						},
					}
					u := NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, nil)
					var err error
					if operation == "update" {
						_, err = u.Update(context.Background(), tc.principal, UpdateCommentCommand{
							SubjectType: subjectType, CommentID: 7, Content: "updated",
						})
					} else {
						err = u.Delete(
							context.Background(),
							tc.principal,
							DeleteCommentCommand{
								SubjectType: subjectType,
								CommentID:   7,
							},
						)
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
	u := NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, nil)
	actor := Actor{UserID: 42, Username: "alice", CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}

	if _, err := u.Create(
		context.Background(),
		actor,
		CreatePostCommentCommand{PostID: 7, Content: "body"},
	); err != nil {
		t.Fatal(err)
	}
	if repo.input.AuthorID != actor.UserID || repo.input.AuthorUsername != actor.Username {
		t.Fatalf("persisted author = %d/%q, want %d/%q",
			repo.input.AuthorID, repo.input.AuthorUsername, actor.UserID, actor.Username)
	}
}

func TestExternalCommentCheckFailureStopsWrite(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(fmt.Sprintf("exists=%t", exists), func(t *testing.T) {
			repo := &commentRepoStub{}
			checks := 0
			cause := errors.New("private upstream failure")
			u := NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, subjectResolverStub{
				supported: true, valid: true, exists: exists, checkErr: cause, checks: &checks,
			})
			comment, err := u.CreateExternal(context.Background(), Actor{UserID: 1, CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}, CreateExternalCommentCommand{
				Kind: "novel", SubjectKey: "web-syosetu-n1234", Content: "body",
			})
			var appErr *AppError
			if err == nil || errors.As(err, &appErr) || comment != nil || repo.writes != 0 || checks != 1 {
				t.Fatalf("comment=%v err=%v writes=%d checks=%d", comment, err, repo.writes, checks)
			}
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), "comment.check_subject") || !strings.Contains(err.Error(), cause.Error()) {
				t.Fatalf("check failure lost its operation or original cause: %v", err)
			}
		})
	}
}

func TestExternalCommentChecksBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name      string
		supported bool
		valid     bool
		exists    bool
		wantCode  string
	}{
		{"exists", true, true, true, ""},
		{"missing", true, true, false, CodeCommentSubjectNotFound},
		{"invalid", true, false, false, CodeCommentSubjectKeyInvalid},
		{"unsupported", false, false, false, CodeCommentSubjectTypeInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &commentRepoStub{}
			checks := 0
			checker := subjectResolverStub{
				supported: tc.supported,
				valid:     tc.valid,
				exists:    tc.exists,
				checks:    &checks,
			}
			u := NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, checker)
			actor := Actor{UserID: 1, Username: "tester", CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}
			command := CreateExternalCommentCommand{
				Kind: "novel", SubjectKey: "web-syosetu-n1234", Content: "body",
			}
			_, err := u.CreateExternal(context.Background(), actor, command)
			wantChecks := 0
			if tc.supported && tc.valid {
				wantChecks = 1
			}
			if checks != wantChecks {
				t.Fatalf("checks=%d want=%d", checks, wantChecks)
			}
			if tc.wantCode == "" {
				if err != nil || repo.writes != 1 ||
					repo.input.SubjectType != domain.CommentSubjectNovel ||
					repo.input.AuthorID != actor.UserID ||
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
		})
	}
}

type failingCommentRepository struct {
	repository.CommentRepository
	err     error
	root    *domain.Comment
	lockErr error
}

func (r failingCommentRepository) Create(_ context.Context, input domain.Comment) (*domain.Comment, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &input, nil
}
func (r failingCommentRepository) Lock(_ context.Context, kind domain.CommentSubjectType, id int64) (*domain.Comment, error) {
	if r.lockErr != nil {
		return nil, r.lockErr
	}
	if r.root != nil {
		copy := *r.root
		return &copy, nil
	}
	return &domain.Comment{ID: id, SubjectType: kind, SubjectKey: "42"}, nil
}

func (r failingCommentRepository) Find(ctx context.Context, kind domain.CommentSubjectType, id int64) (*domain.Comment, error) {
	return r.Lock(ctx, kind, id)
}

func (r failingCommentRepository) ListReplies(
	context.Context,
	domain.CommentSubjectType,
	string,
	int64,
	int64,
	int64,
) (int64, []domain.Comment, error) {
	return 0, nil, r.err
}

func TestCommentRepositoryErrors(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) bool {
		return true
	})
	for _, tc := range []struct {
		name         string
		call         func(*CommentUsecase) error
		missingCode  string
		storageError error
	}{
		{"comment.create_post", func(u *CommentUsecase) error {
			_, err := u.Create(context.Background(), Actor{CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}, CreatePostCommentCommand{PostID: 42, Content: "body"})
			return err
		}, CodeCommentSubjectNotFound, repository.ErrNotFound},
		{"comment.create_external", func(u *CommentUsecase) error {
			_, err := u.CreateExternal(context.Background(), Actor{CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}, CreateExternalCommentCommand{
				Kind: "novel", SubjectKey: "wenku-book", RootID: int64Pointer(7), Content: "body",
			})
			return err
		}, CodeCommentRootNotFound, repository.ErrNotFound},
		{"comment.list_replies", func(u *CommentUsecase) error {
			_, _, err := u.ListReplies(context.Background(), Actor{}, ListCommentRepliesQuery{
				SubjectType: domain.CommentSubjectNovel, SubjectKey: "wenku-book", RootID: 7, Limit: 20,
			})
			return err
		}, CodeCommentRootNotFound, repository.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("private database details")
			u := NewCommentUsecase(immediateTransaction{}, failingCommentRepository{err: cause, root: &domain.Comment{ID: 7, SubjectType: domain.CommentSubjectNovel, SubjectKey: "wenku-book"}}, postExistenceStub{exists: true}, nil, checker)
			err := tc.call(u)
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("expected operation and original cause, got %v", err)
			}
			missing := fmt.Errorf("query: %w", tc.storageError)
			repo := failingCommentRepository{err: missing}
			post := postExistenceStub{exists: true}
			if tc.name == "comment.create_post" {
				post.err = missing
				repo.err = nil
			}
			if tc.name == "comment.create_external" {
				repo.err = nil
				repo.lockErr = missing
			}
			u = NewCommentUsecase(immediateTransaction{}, repo, post, nil, checker)
			if err := tc.call(u); !isAppErrorCode(err, tc.missingCode) {
				t.Fatalf("expected code %s, got %v", tc.missingCode, err)
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }

type postExistenceStub struct {
	exists bool
	err    error
	locked bool
}

func (s postExistenceStub) Lock(_ context.Context, id int64) (*domain.Post, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.exists {
		return nil, repository.ErrNotFound
	}
	return &domain.Post{ID: id, CommentsLocked: s.locked}, nil
}
func (s postExistenceStub) AdjustCommentsCount(context.Context, int64, int32, *time.Time) error {
	return s.err
}

func (s postExistenceStub) ExistsPublished(context.Context, int64) (bool, error) {
	return s.exists, s.err
}

func TestCommentPublishedPostCheck(t *testing.T) {
	u := NewCommentUsecase(immediateTransaction{}, nil, postExistenceStub{exists: true}, nil, nil)
	if err := u.checkSubjectExist(context.Background(), domain.CommentSubjectPost, "42"); err != nil {
		t.Fatalf("published post rejected: %v", err)
	}
	cause := errors.New("storage unavailable")
	u = NewCommentUsecase(immediateTransaction{}, nil, postExistenceStub{err: cause}, nil, nil)
	if err := u.checkSubjectExist(context.Background(), domain.CommentSubjectPost, "42"); !errors.Is(err, cause) {
		t.Fatalf("storage error lost: %v", err)
	}
}

func TestCommentReadSubjectChecks(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) bool {
		t.Fatal("reads must not check external resources")
		return false
	})
	// Unimplemented comment queries panic if a missing post reaches the repository.
	missing := NewCommentUsecase(immediateTransaction{},
		&struct{ repository.CommentRepository }{},
		postExistenceStub{},
		nil,
		checker,
	)
	external := NewCommentUsecase(immediateTransaction{}, &commentRepoStub{}, nil, nil, checker)
	for _, replies := range []bool{false, true} {
		var err error
		if replies {
			_, _, err = missing.ListReplies(context.Background(), Actor{}, ListCommentRepliesQuery{
				SubjectType: domain.CommentSubjectPost, SubjectKey: "42", RootID: 7, Limit: 20,
			})
		} else {
			_, _, err = missing.List(context.Background(), Actor{}, ListCommentsQuery{
				SubjectType: domain.CommentSubjectPost, SubjectKey: "42", Limit: 20,
			})
		}
		if !isAppErrorCode(err, CodeCommentSubjectNotFound) {
			t.Fatalf("expected missing post, got %v", err)
		}
		if replies {
			_, _, err = external.ListReplies(context.Background(), Actor{}, ListCommentRepliesQuery{
				SubjectType: domain.CommentSubjectNovel, SubjectKey: "deleted", RootID: 7, Limit: 20,
			})
		} else {
			_, _, err = external.List(context.Background(), Actor{}, ListCommentsQuery{
				SubjectType: domain.CommentSubjectNovel, SubjectKey: "deleted", Limit: 20,
			})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommentCreationStorageFailures(t *testing.T) {
	checker := subjectCheckFunc(func(context.Context, string, string) bool { return true })
	for _, external := range []bool{false, true} {
		for _, tc := range []struct {
			name, wantCode string
			postOnly       bool
		}{
			{"missing root", CodeCommentRootNotFound, false},
			{"invalid root", CodeCommentRootInvalid, false},
			{"conflict", CodeCommentConflict, false},
			{"missing post", CodeCommentSubjectNotFound, true},
			{"locked post", CodeCommentLocked, true},
		} {
			t.Run(fmt.Sprintf("external=%t/%s", external, tc.name), func(t *testing.T) {
				kind := domain.CommentSubjectPost
				if external {
					kind = domain.CommentSubjectNovel
				}
				repo := failingCommentRepository{root: &domain.Comment{ID: 7, SubjectType: kind, SubjectKey: "42"}}
				post := postExistenceStub{exists: true}
				switch tc.name {
				case "missing root":
					repo.lockErr = fmt.Errorf("find root: %w", repository.ErrNotFound)
				case "invalid root":
					repo.root.SubjectKey = "another subject"
				case "conflict":
					repo.err = fmt.Errorf("create: %w", repository.ErrConflict)
				case "missing post":
					post.exists = false
				case "locked post":
					post.locked = true
				}
				u := NewCommentUsecase(immediateTransaction{}, repo, post, nil, checker)
				rootID := int64(7)
				var err error
				if external {
					_, err = u.CreateExternal(context.Background(), Actor{CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}, CreateExternalCommentCommand{Kind: "novel", SubjectKey: "42", RootID: &rootID, Content: "reply"})
				} else {
					_, err = u.Create(context.Background(), Actor{CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}, CreatePostCommentCommand{PostID: 42, RootID: &rootID, Content: "reply"})
				}
				if external && tc.postOnly {
					if err != nil {
						t.Fatalf("external comments must not inherit post constraints: %v", err)
					}
					return
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
		for _, status := range []domain.CommentStatus{
			domain.CommentStatusPublished, domain.CommentStatusHidden, domain.CommentStatusDeleted, 99,
		} {
			t.Run(fmt.Sprintf("actor=%+v/status=%d", actor, status), func(t *testing.T) {
				repo := &commentRepoStub{comment: domain.Comment{ID: 7, AuthorID: 1, Content: "body", Status: status}}
				u := NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, nil)
				want := repo.comment
				if !actor.IsAdmin && status != domain.CommentStatusPublished {
					want.Content = ""
				}
				total, roots, err := u.List(context.Background(), actor, ListCommentsQuery{
					SubjectType: domain.CommentSubjectNovel, SubjectKey: "book", Limit: 20,
				})
				if err != nil || total != 1 || len(roots) != 1 {
					t.Fatalf("roots=%+v total=%d err=%v", roots, total, err)
				}
				if roots[0].Root != want || roots[0].ReplyCount != 1 || len(roots[0].Replies) != 1 || roots[0].Replies[0] != want {
					t.Fatalf("unexpected thread: %+v", roots[0])
				}
				total, replies, err := u.ListReplies(context.Background(), actor, ListCommentRepliesQuery{
					SubjectType: domain.CommentSubjectNovel, SubjectKey: "book", RootID: 7, Limit: 20,
				})
				if err != nil || total != 1 || len(replies) != 1 || replies[0] != want {
					t.Fatalf("replies=%+v total=%d err=%v", replies, total, err)
				}
			})
		}
	}
}

func TestCommentCreationRequiresThirtyDayOldAccount(t *testing.T) {
	for _, external := range []bool{false, true} {
		for _, admin := range []bool{false, true} {
			for _, tc := range []struct {
				name      string
				createdAt time.Time
				allowed   bool
			}{
				{"missing", time.Time{}, false},
				{"future", time.Now().Add(time.Hour), false},
				{"under thirty days", time.Now().Add(-30*24*time.Hour + time.Minute), false},
				{"thirty days", time.Now().Add(-30 * 24 * time.Hour), true},
				{"older", time.Now().Add(-31 * 24 * time.Hour), true},
			} {
				rootID := int64(7)
				for _, root := range []*int64{nil, &rootID} {
					t.Run(fmt.Sprintf("external=%t/admin=%t/%s/reply=%t", external, admin, tc.name, root != nil), func(t *testing.T) {
						actor := Actor{UserID: 1, IsAdmin: admin, CreatedAt: tc.createdAt}
						// Denied calls must stop before any repository or resolver access.
						u := NewCommentUsecase(immediateTransaction{}, nil, nil, nil, nil)
						repo := &commentRepoStub{comment: domain.Comment{ID: rootID, SubjectKey: "42", SubjectType: domain.CommentSubjectPost}}
						if external {
							repo.comment.SubjectType = domain.CommentSubjectNovel
						}
						if tc.allowed {
							u = NewCommentUsecase(immediateTransaction{}, repo, postExistenceStub{exists: true}, nil, subjectResolverStub{supported: true, valid: true, exists: true})
						}
						var err error
						if external {
							_, err = u.CreateExternal(context.Background(), actor, CreateExternalCommentCommand{Kind: "novel", SubjectKey: "42", RootID: root, Content: "body"})
						} else {
							_, err = u.Create(context.Background(), actor, CreatePostCommentCommand{PostID: 42, RootID: root, Content: "body"})
						}
						if tc.allowed {
							if err != nil || repo.writes != 1 {
								t.Fatalf("err=%v writes=%d", err, repo.writes)
							}
						} else {
							var appErr *AppError
							if !errors.As(err, &appErr) || appErr.Kind != KindPermissionDenied || appErr.Code != CodeCommentAccountTooYoung {
								t.Fatalf("expected account age permission error, got %v", err)
							}
						}
					})
				}
			}
		}
	}
}
