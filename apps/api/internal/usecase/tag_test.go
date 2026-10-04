package usecase

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"forum/internal/domain"
	"forum/internal/repository"
)

type tagStub struct {
	repository.TagRepository
	called         bool
	categoryID, id int64
	name, attr     string
	color          int16
	sortOrder      int32
	active         bool
	err            error
}

func (r *tagStub) ListActive() ([]domain.Tag, error) {
	r.called = true
	return []domain.Tag{{ID: 9, CategoryID: 1, IsActive: true}}, r.err
}
func (r *tagStub) ListByCategory(cid int64) ([]domain.Tag, error) {
	r.called, r.categoryID = true, cid
	return []domain.Tag{{ID: 9, CategoryID: cid}}, r.err
}
func (r *tagStub) Create(cid int64, name string, color int16, order int32, attr string) (*domain.Tag, error) {
	r.called, r.categoryID, r.name, r.color, r.sortOrder, r.attr = true, cid, name, color, order, attr
	return &domain.Tag{ID: 9, CategoryID: cid, Name: name, Color: color, SortOrder: order}, r.err
}
func (r *tagStub) Update(cid, id int64, name string, color int16, order int32) (*domain.Tag, error) {
	tag, err := r.Create(cid, name, color, order, "")
	r.id = id
	return tag, err
}
func (r *tagStub) SetActive(cid, id int64, active bool) error {
	r.called, r.categoryID, r.id, r.active = true, cid, id, active
	return r.err
}

func assertTagCode(t *testing.T, err error, code string) {
	t.Helper()
	var app *AppError
	if !errors.As(err, &app) || app.Code != code {
		t.Fatalf("error=%v want code=%s", err, code)
	}
}

func TestTagUsecaseAuthorization(t *testing.T) {
	for _, actor := range []Actor{{}, {UserID: 1}} {
		for _, operation := range []string{"list", "create", "update", "active"} {
			t.Run(fmt.Sprintf("%s/%d", operation, actor.UserID), func(t *testing.T) {
				repo := &tagStub{}
				u := NewTagUsecase(repo)
				var err error
				switch operation {
				case "list":
					_, err = u.ListAdmin(actor, ListTagsQuery{CategoryID: 1})
				case "create":
					_, err = u.Create(actor, TagInput{CategoryID: 1, Name: "标签"})
				case "update":
					_, err = u.Update(actor, 9, TagInput{CategoryID: 1, Name: "标签"})
				case "active":
					err = u.SetActive(actor, SetTagActiveCommand{CategoryID: 1, ID: 9, Active: true})
				}
				assertTagCode(t, err, CodeTagAdminRequired)
				if repo.called {
					t.Fatal("unauthorized call reached repository")
				}
			})
		}
	}
}

func TestTagInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input TagInput
		code  string
	}{
		{"zero category", TagInput{Name: "标签"}, CodeTagCategoryInvalid},
		{"unknown category", TagInput{CategoryID: 999, Name: "标签"}, CodeTagCategoryNotFound},
		{"empty name", TagInput{CategoryID: 1}, CodeTagNameInvalid},
		{"whitespace", TagInput{CategoryID: 1, Name: " \t\n　"}, CodeTagNameInvalid},
		{"long name", TagInput{CategoryID: 1, Name: strings.Repeat("标", 65)}, CodeTagNameInvalid},
		{"negative color", TagInput{CategoryID: 1, Name: "标签", Color: -1}, CodeTagColorInvalid},
	} {
		for _, update := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, update), func(t *testing.T) {
				repo := &tagStub{}
				u := NewTagUsecase(repo)
				var err error
				if update {
					_, err = u.Update(Actor{IsAdmin: true}, 9, tc.input)
				} else {
					_, err = u.Create(Actor{IsAdmin: true}, tc.input)
				}
				assertTagCode(t, err, tc.code)
				if repo.called {
					t.Fatal("invalid input reached repository")
				}
			})
		}
	}
}

func TestTagMutations(t *testing.T) {
	repo := &tagStub{}
	u := NewTagUsecase(repo)
	admin := Actor{IsAdmin: true}
	input := TagInput{CategoryID: 1, Name: "　" + strings.Repeat("标", 64) + " ", Color: 2, SortOrder: -3}
	tag, err := u.Create(admin, input)
	if err != nil || tag.Name != strings.TrimSpace(input.Name) || repo.attr != "{}" || repo.categoryID != 1 || repo.color != 2 || repo.sortOrder != -3 {
		t.Fatalf("create: tag=%+v repo=%+v err=%v", tag, repo, err)
	}
	_, err = u.Update(admin, 9, input)
	if err != nil || repo.id != 9 || repo.name != strings.TrimSpace(input.Name) {
		t.Fatalf("update: repo=%+v err=%v", repo, err)
	}
	for _, active := range []bool{true, false} {
		err = u.SetActive(admin, SetTagActiveCommand{CategoryID: 1, ID: 9, Active: active})
		if err != nil || repo.active != active || repo.categoryID != 1 || repo.id != 9 {
			t.Fatalf("active: repo=%+v err=%v", repo, err)
		}
	}
}

func TestTagQueryAndIDValidation(t *testing.T) {
	admin := Actor{IsAdmin: true}
	for _, tc := range []struct {
		cid, id int64
		code    string
	}{
		{0, 9, CodeTagCategoryInvalid}, {999, 9, CodeTagCategoryNotFound}, {1, 0, CodeTagIDInvalid}, {1, -1, CodeTagIDInvalid},
	} {
		repo := &tagStub{}
		u := NewTagUsecase(repo)
		err := u.SetActive(admin, SetTagActiveCommand{CategoryID: tc.cid, ID: tc.id})
		assertTagCode(t, err, tc.code)
		if repo.called {
			t.Fatal("invalid command reached repository")
		}
	}
	for _, cid := range []int64{0, 999} {
		repo := &tagStub{}
		_, err := NewTagUsecase(repo).ListAdmin(admin, ListTagsQuery{CategoryID: cid})
		if err == nil || repo.called {
			t.Fatal("invalid query accepted")
		}
	}
	repo := &tagStub{}
	_, err := NewTagUsecase(repo).Update(admin, 0, TagInput{CategoryID: 1, Name: "标签"})
	assertTagCode(t, err, CodeTagIDInvalid)
	if repo.called {
		t.Fatal("invalid update reached repository")
	}
	tags, err := NewTagUsecase(repo).ListActive()
	if err != nil || len(tags) != 1 || !tags[0].IsActive {
		t.Fatalf("active list=%+v err=%v", tags, err)
	}
	tags, err = NewTagUsecase(repo).ListAdmin(admin, ListTagsQuery{CategoryID: 2})
	if err != nil || len(tags) != 1 || repo.categoryID != 2 {
		t.Fatalf("admin list=%+v err=%v", tags, err)
	}
}

func TestTagErrors(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, operation := range []string{"create", "update", "active"} {
		for _, tc := range []struct {
			err  error
			code string
		}{
			{fmt.Errorf("wrapped: %w", repository.ErrConflict), CodeTagConflict},
			{fmt.Errorf("wrapped: %w", repository.ErrNotFound), CodeTagNotFound},
			{failure, ""},
		} {
			t.Run(operation+"/"+tc.err.Error(), func(t *testing.T) {
				u := NewTagUsecase(&tagStub{err: tc.err})
				admin := Actor{IsAdmin: true}
				var err error
				switch operation {
				case "create":
					_, err = u.Create(admin, TagInput{CategoryID: 1, Name: "标签"})
				case "update":
					_, err = u.Update(admin, 9, TagInput{CategoryID: 1, Name: "标签"})
				case "active":
					err = u.SetActive(admin, SetTagActiveCommand{CategoryID: 1, ID: 9})
				}
				if tc.code != "" {
					assertTagCode(t, err, tc.code)
				} else {
					var app *AppError
					if !errors.Is(err, failure) || errors.As(err, &app) {
						t.Fatalf("infrastructure error lost: %v", err)
					}
				}
			})
		}
	}
	u := NewTagUsecase(&tagStub{err: failure})
	_, err := u.ListActive()
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	_, err = u.ListAdmin(Actor{IsAdmin: true}, ListTagsQuery{CategoryID: 1})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
}
