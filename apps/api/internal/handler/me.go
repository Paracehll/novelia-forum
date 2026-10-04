package handler

import (
	"net/http"

	"forum/internal/httpx"
	"forum/internal/repository"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
)

type meHandler struct {
	postUsecase *usecase.PostUsecase
}

func NewMeHandler(posts *usecase.PostUsecase) *meHandler {
	return &meHandler{postUsecase: posts}
}

func (h *meHandler) RegisterRoutes(router chi.Router) {
	router.Use(httpx.RequireAccessToken)
	router.Get("/post", httpx.EH(h.listPosts))
	router.Get("/favorite", httpx.EH(h.listFavorites))
}

func (h *meHandler) listPosts(w http.ResponseWriter, r *http.Request) error {
	return respondPosts(w, r, h.postUsecase.ListMine, repository.PostFilter{})
}

func (h *meHandler) listFavorites(w http.ResponseWriter, r *http.Request) error {
	return respondPosts(w, r, h.postUsecase.ListFavorites, repository.PostFilter{})
}
