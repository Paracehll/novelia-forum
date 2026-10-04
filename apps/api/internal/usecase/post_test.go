package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	forumcategory "forum/internal/category"
	"forum/internal/domain"
	"forum/internal/repository"
)

type postUsecaseRepoStub struct {
	repository.PostRepository
	post           domain.Post
	listItem       domain.PostListItem
	filter         repository.PostFilter
	limit, offset  int64
	input          repository.CreatePostInput
	writes         int
	incrementViews bool
	err            error
	updateErr      error
}

func (r *postUsecaseRepoStub) List(
	ctx context.Context,
	filter repository.PostFilter,
	limit, offset int64,
) (int64, []domain.PostListItem, error) {
	r.filter = filter
	r.limit, r.offset = limit, offset
	return 1, []domain.PostListItem{r.listItem}, r.err
}
func (r *postUsecaseRepoStub) Find(ctx context.Context, _ int64, increment bool) (*domain.Post, error) {
	r.incrementViews = increment
	return &r.post, r.err
}
func (r *postUsecaseRepoStub) Create(ctx context.Context, input repository.CreatePostInput) (*domain.Post, error) {
	r.input = input
	r.writes++
	return &r.post, r.err
}
func (r *postUsecaseRepoStub) Update(
	ctx context.Context,
	_ int64,
	input repository.UpdatePostInput,
) (*domain.Post, error) {
	r.input.Title, r.input.Content = input.Title, input.Content
	r.writes++
	return &r.post, r.updateErr
}
func (r *postUsecaseRepoStub) SetStatus(ctx context.Context, _ int64, status domain.PostStatus) error {
	r.writes++
	r.post.Status = status
	return r.err
}

type postUsecaseFavoriteStub struct {
	repository.FavoriteRepository
	userID int64
	err    error
}

func (r *postUsecaseFavoriteStub) ListPostIDs(
	ctx context.Context,
	userID int64,
	ids []int64,
) (map[int64]bool, error) {
	r.userID = userID
	favorites := map[int64]bool{}
	for _, id := range ids {
		favorites[id] = true
	}
	return favorites, r.err
}
func (r *postUsecaseFavoriteStub) Has(ctx context.Context, _ int64, userID int64) (bool, error) {
	r.userID = userID
	return true, r.err
}

func TestPostListQueryMapsToRepositoryFilter(t *testing.T) {
	status := domain.PostStatusHidden
	repo := &postUsecaseRepoStub{}
	u := NewPostUsecase(immediateTransaction{}, repo, nil, nil)
	query := ListPostsQuery{
		CategorySlug: "discussion", Search: " title ", Sort: PostSortNewest,
		TagIDs: []int64{2, 3}, AuthorName: " alice ", AuthorID: 7,
		Status: &status, Limit: 25, Offset: 50,
	}
	if _, _, err := u.ListAdmin(context.Background(), Actor{IsAdmin: true}, query); err != nil {
		t.Fatal(err)
	}
	filter := repo.filter
	if filter.CategorySlug != "discussion" || filter.Search != "title" || filter.Sort != repository.PostSortNewest ||
		len(filter.TagIDs) != 2 || filter.TagIDs[0] != 2 || filter.TagIDs[1] != 3 ||
		filter.AuthorName != "alice" || filter.AuthorID != 7 || filter.Status != status ||
		filter.FavoriteUserID != 0 || repo.limit != 25 || repo.offset != 50 {
		t.Fatalf("query mapping lost fields: filter=%+v limit=%d offset=%d", filter, repo.limit, repo.offset)
	}
	if query.Search != " title " || query.AuthorName != " alice " || *query.Status != domain.PostStatusHidden {
		t.Fatal("mutated caller query")
	}
}

func TestPostListQueryDefaultsAndValidation(t *testing.T) {
	invalidStatus := domain.PostStatus(3)
	for _, tc := range []struct {
		name  string
		query ListPostsQuery
		code  string
	}{
		{"defaults", ListPostsQuery{Limit: 20}, ""},
		{"invalid status", ListPostsQuery{Limit: 20, Status: &invalidStatus}, CodePostStatusInvalid},
		{"invalid author", ListPostsQuery{Limit: 20, AuthorID: -1}, CodePostAuthorInvalid},
		{"invalid sort", ListPostsQuery{Limit: 20, Sort: "invalid"}, CodePostSortInvalid},
		{"duplicate tags", ListPostsQuery{Limit: 20, TagIDs: []int64{1, 1}}, CodePostTagInvalid},
		{"invalid limit", ListPostsQuery{Limit: 101}, CodePostPaginationInvalid},
		{"invalid offset", ListPostsQuery{Limit: 20, Offset: -1}, CodePostPaginationInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &postUsecaseRepoStub{}
			u := NewPostUsecase(immediateTransaction{}, repo, nil, nil)
			_, _, err := u.ListAdmin(context.Background(), Actor{IsAdmin: true}, tc.query)
			if tc.code != "" {
				if !isAppErrorCode(err, tc.code) {
					t.Fatalf("error=%v want code=%s", err, tc.code)
				}
				return
			}
			if err != nil || repo.filter.Status != repository.PostStatusAll || repo.filter.Sort != repository.PostSortActive {
				t.Fatalf("defaults: filter=%+v error=%v", repo.filter, err)
			}
		})
	}
}

func TestPostListScopes(t *testing.T) {
	for _, mode := range []string{"public", "mine", "favorites", "admin"} {
		t.Run(mode, func(t *testing.T) {
			repo := &postUsecaseRepoStub{listItem: domain.PostListItem{ID: 5}}
			favorites := &postUsecaseFavoriteStub{}
			u := NewPostUsecase(immediateTransaction{}, repo, favorites, nil)
			actor := Actor{UserID: 7, IsAdmin: true}
			status := domain.PostStatusHidden
			query := ListPostsQuery{Limit: 20, Status: &status, AuthorName: "someone", AuthorID: 99}
			var total int64
			var items []domain.PostListItem
			var err error
			switch mode {
			case "public":
				total, items, err = u.List(context.Background(), actor, query)
			case "mine":
				total, items, err = u.ListMine(context.Background(), actor, query)
			case "favorites":
				total, items, err = u.ListFavorites(context.Background(), actor, query)
			case "admin":
				total, items, err = u.ListAdmin(context.Background(), actor, query)
			}
			if err != nil || total != 1 || len(items) != 1 || !items[0].Favorited || favorites.userID != 7 {
				t.Fatalf("total=%d items=%v err=%v", total, items, err)
			}
			wantAuthor, wantFavorite := int64(0), int64(0)
			if mode == "mine" {
				wantAuthor = 7
			}
			if mode == "favorites" {
				wantFavorite = 7
			}
			if mode == "admin" {
				if repo.filter.Status != domain.PostStatusHidden || repo.filter.AuthorID != 99 || repo.filter.FavoriteUserID != 0 || repo.filter.AuthorName != "someone" {
					t.Fatalf("admin filter lost: %+v", repo.filter)
				}
			} else if repo.filter.Status != domain.PostStatusPublished || repo.filter.AuthorName != "" || repo.filter.AuthorID != wantAuthor || repo.filter.FavoriteUserID != wantFavorite {
				t.Fatalf("unsafe filter: %+v", repo.filter)
			}
			if query.AuthorID != 99 || query.Status != &status || status != domain.PostStatusHidden {
				t.Fatal("mutated caller query")
			}
		})
	}
	u := NewPostUsecase(immediateTransaction{}, nil, nil, nil)
	if _, _, err := u.ListMine(
		context.Background(),
		Actor{},
		ListPostsQuery{},
	); !isAppErrorCode(err, CodePostAuthRequired) {
		t.Fatal(err)
	}
	if _, _, err := u.ListFavorites(
		context.Background(),
		Actor{},
		ListPostsQuery{},
	); !isAppErrorCode(err, CodePostAuthRequired) {
		t.Fatal(err)
	}
	if _, _, err := u.ListAdmin(
		context.Background(),
		Actor{},
		ListPostsQuery{},
	); !isAppErrorCode(err, CodePostAdminRequired) {
		t.Fatal(err)
	}
	if err := u.SetStatus(context.Background(), Actor{}, 1, 0); !isAppErrorCode(err, CodePostAdminRequired) {
		t.Fatal(err)
	}
	if err := u.SetCommentsLocked(
		context.Background(),
		Actor{},
		1,
		true,
	); !isAppErrorCode(err, CodePostAdminRequired) {
		t.Fatal(err)
	}
	if err := u.SetPinOrder(context.Background(), Actor{}, 1, nil); !isAppErrorCode(err, CodePostAdminRequired) {
		t.Fatal(err)
	}
}

func TestPostInputAndModificationRules(t *testing.T) {
	repo := &postUsecaseRepoStub{post: domain.Post{ID: 1, AuthorID: 7, CreatedAt: time.Now().Add(-time.Hour)}}
	u := NewPostUsecase(immediateTransaction{}, repo, &postUsecaseFavoriteStub{}, nil)
	actor := Actor{UserID: 7, Username: "author"}
	input := PostInput{CategoryID: forumcategory.NovelID, Title: " 标题 ", Content: " 内容 \n"}
	result, err := u.Create(context.Background(), actor, input)
	if err != nil || result.Favorited || repo.input.Title != "标题" || repo.input.Content != input.Content || repo.input.AuthorID != 7 || repo.input.AuthorUsername != "author" || repo.input.Attr != "{}" {
		t.Fatalf("input=%+v result=%v err=%v", repo.input, result, err)
	}
	if _, err := u.Update(context.Background(), actor, 1, input); err != nil {
		t.Fatalf("old post must remain editable: %v", err)
	}
	if err := u.Delete(context.Background(), actor, 1); !isAppErrorCode(err, CodePostDeleteExpired) {
		t.Fatal(err)
	}
	if _, err := u.Update(context.Background(), Actor{UserID: 8}, 1, input); !isAppErrorCode(err, CodePostNotOwner) {
		t.Fatal(err)
	}
	if err := u.Delete(
		context.Background(),
		Actor{IsAdmin: true},
		1,
	); err != nil || repo.post.Status != domain.PostStatusDeleted {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		input PostInput
		code  string
	}{
		{PostInput{CategoryID: 0, Title: "标题", Content: "内容"}, CodePostCategoryInvalid},
		{PostInput{CategoryID: forumcategory.NovelID, Title: "字", Content: "内容"}, CodePostTitleInvalid},
		{PostInput{CategoryID: forumcategory.NovelID, Title: "标题", Content: " \n"}, CodePostContentInvalid},
		{PostInput{CategoryID: forumcategory.NovelID, Title: "标题", Content: strings.Repeat("字", 20000) + " "}, CodePostContentInvalid},
		{PostInput{CategoryID: forumcategory.NovelID, Title: "标题", Content: "内容", TagIDs: []int64{1, 1}}, CodePostTagInvalid},
		{PostInput{CategoryID: forumcategory.NovelID, Title: "标题", Content: "内容", TagIDs: []int64{1, 2, 3, 4}}, CodePostTagInvalid},
		{PostInput{CategoryID: forumcategory.AnnouncementsID, Title: "标题", Content: "内容"}, CodePostAnnouncementRestricted},
	} {
		writes := repo.writes
		if _, err := u.Create(
			context.Background(),
			actor,
			tc.input,
		); !isAppErrorCode(err, tc.code) || writes != repo.writes {
			t.Fatalf("input=%+v err=%v", tc.input, err)
		}
	}
	if _, err := u.Get(context.Background(), Actor{}, 1); err != nil || !repo.incrementViews {
		t.Fatalf("get err=%v increment=%v", err, repo.incrementViews)
	}
}

func TestPostRepositoryErrors(t *testing.T) {
	input := PostInput{CategoryID: forumcategory.NovelID, Title: "标题", Content: "正文"}
	for _, tc := range []struct {
		cause error
		kind  ErrorKind
		code  string
	}{
		{repository.ErrNotFound, KindNotFound, CodePostNotFound},
		{repository.ErrInvalidCategory, KindInvalid, CodePostCategoryInvalid},
		{repository.ErrInvalidTag, KindInvalid, CodePostTagInvalid},
		{repository.ErrConflict, KindConflict, CodePostConflict},
	} {
		u := NewPostUsecase(immediateTransaction{}, &postUsecaseRepoStub{err: fmt.Errorf("repo: %w", tc.cause)}, nil, nil)
		var err error
		if tc.cause == repository.ErrNotFound {
			_, err = u.Get(context.Background(), Actor{}, 1)
		} else {
			_, err = u.Create(context.Background(), Actor{UserID: 7}, input)
		}
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Kind != tc.kind || appErr.Code != tc.code {
			t.Fatalf("cause=%v err=%v", tc.cause, err)
		}
	}
	repo := &postUsecaseRepoStub{post: domain.Post{ID: 1, AuthorID: 7}, updateErr: fmt.Errorf("repo: %w", repository.ErrNotFound)}
	u := NewPostUsecase(immediateTransaction{}, repo, nil, nil)
	_, err := u.Update(context.Background(), Actor{UserID: 7}, 1, input)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Kind != KindConflict || appErr.Code != CodePostConflict {
		t.Fatalf("post changed before update: %v", err)
	}
}

func TestPostReadErrorsKeepCause(t *testing.T) {
	for _, cause := range []error{repository.ErrNotFound, repository.ErrInvalidTag, errors.New("storage unavailable")} {
		t.Run(cause.Error(), func(t *testing.T) {
			repo := &postUsecaseRepoStub{err: cause}
			u := NewPostUsecase(immediateTransaction{}, repo, nil, nil)
			_, _, listErr := u.List(context.Background(), Actor{}, ListPostsQuery{Limit: 20})
			repo.err = nil
			u.favoriteRepo = &postUsecaseFavoriteStub{err: cause}
			_, _, favoritesErr := u.List(context.Background(), Actor{UserID: 7}, ListPostsQuery{Limit: 20})
			_, favoriteErr := u.Get(context.Background(), Actor{UserID: 7}, 1)
			for _, err := range []error{listErr, favoritesErr, favoriteErr} {
				var appErr *AppError
				if !errors.Is(err, cause) || errors.As(err, &appErr) {
					t.Fatalf("unexpected read error conversion: cause=%v err=%v", cause, err)
				}
			}
		})
	}
}
