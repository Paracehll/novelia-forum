package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"forum/internal/domain"
	"forum/internal/repository"
	"forum/internal/usecase"
)

// immediateTransaction keeps existing handler tests independent of SQL transactions.
type immediateTransaction struct{}

func (immediateTransaction) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type requestContextPostRepository struct {
	repository.PostRepository
	ctx context.Context
}

func (r *requestContextPostRepository) List(
	ctx context.Context,
	_ repository.PostFilter,
	_, _ int64,
) (int64, []domain.PostListItem, error) {
	r.ctx = ctx
	return 0, nil, nil
}

func TestPostHandlerPropagatesRequestContext(t *testing.T) {
	type requestKey struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), requestKey{}, "request-value"), time.Minute)
	defer cancel()
	repo := &requestContextPostRepository{}
	h := NewPostHandler(usecase.NewPostUsecase(immediateTransaction{}, repo, availableTagRepository{}, nil, nil), nil)
	request := httptest.NewRequest(http.MethodGet, "/post/", nil).WithContext(ctx)
	if err := h.list(httptest.NewRecorder(), request); err != nil {
		t.Fatal(err)
	}
	if repo.ctx != request.Context() || repo.ctx.Value(requestKey{}) != "request-value" {
		t.Fatal("handler/usecase replaced the request context")
	}
	deadline, ok := repo.ctx.Deadline()
	want, _ := ctx.Deadline()
	if !ok || deadline != want {
		t.Fatal("request deadline lost before reaching repository")
	}
	cancel()
	select {
	case <-repo.ctx.Done():
		if repo.ctx.Err() != context.Canceled {
			t.Fatalf("error=%v", repo.ctx.Err())
		}
	default:
		t.Fatal("request cancellation did not reach repository context")
	}
}
