package handler

import (
	"net/http"
	"time"

	"forum/internal/httpx"
	"forum/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type meHandler struct {
	postUsecase      *usecase.PostUsecase
	blacklistUsecase *usecase.BlacklistUsecase
}

func NewMeHandler(
	posts *usecase.PostUsecase,
	blacklist *usecase.BlacklistUsecase,
) *meHandler {
	return &meHandler{
		postUsecase:      posts,
		blacklistUsecase: blacklist,
	}
}

func (h *meHandler) RegisterRoutes(router chi.Router) {
	router.Use(httpx.RequireAccessToken)
	router.Get("/post", httpx.EH(h.listPosts))
	router.Get("/favorite", httpx.EH(h.listFavorites))
	router.Route("/blacklist", func(router chi.Router) {
		router.Get("/", httpx.EH(h.listBlacklist))
		router.Put("/{userID}", httpx.EH(h.setBlacklist))
		router.Delete("/{userID}", httpx.EH(h.setBlacklist))
	})
}

func (h *meHandler) listPosts(w http.ResponseWriter, r *http.Request) error {
	return respondPosts(w, r, h.postUsecase.ListMine, usecase.ListPostsQuery{})
}

func (h *meHandler) listFavorites(w http.ResponseWriter, r *http.Request) error {
	return respondPosts(w, r, h.postUsecase.ListFavorites, usecase.ListPostsQuery{})
}

type blacklistEntryResponse struct {
	UserID    int64     `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
}

func (h *meHandler) listBlacklist(w http.ResponseWriter, r *http.Request) error {
	principal, err := httpx.AuthenticatedPrincipal(r)
	if err != nil {
		return err
	}
	items, err := h.blacklistUsecase.List(
		r.Context(),
		actorFromPrincipal(principal),
	)
	if err != nil {
		return transportError(err)
	}
	response := page[blacklistEntryResponse]{
		Total: int64(len(items)),
		Items: make([]blacklistEntryResponse, len(items)),
	}
	for i, item := range items {
		response.Items[i] = blacklistEntryResponse{
			UserID:    item.BlockedUserID,
			CreatedAt: item.CreatedAt,
		}
	}
	render.JSON(w, r, response)
	return nil
}

func (h *meHandler) setBlacklist(w http.ResponseWriter, r *http.Request) error {
	principal, err := httpx.AuthenticatedPrincipal(r)
	if err != nil {
		return err
	}
	id, err := httpx.ParseParamPositiveInt(r, "userID")
	if err != nil {
		return err
	}
	err = h.blacklistUsecase.Set(
		r.Context(),
		actorFromPrincipal(principal),
		usecase.SetBlacklistCommand{
			BlockedUserID: id,
			Blocked:       r.Method == http.MethodPut,
		},
	)
	if err != nil {
		return transportError(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
