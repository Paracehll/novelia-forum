package main

import (
	"forum/internal/domainfilter"
	"forum/internal/handler"
	"forum/internal/httpx"
	"forum/internal/infra"
	"forum/internal/repository"
	"forum/internal/subject"
	"forum/internal/usecase"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
)

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return value
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// authn
	httpx.AccessTokenSecret = env("ACCESS_TOKEN_SECRET", "secret")
	file, err := os.Open("domain-blacklist.txt")
	if err != nil {
		slog.Error("Open domain rules failed", "error", err)
		os.Exit(1)
	}
	domains, err := domainfilter.Load(file)
	_ = file.Close()
	if err != nil {
		slog.Error("Load domain rules failed", "error", err)
		os.Exit(1)
	}

	subjects, err := subject.NewRegistry(subject.Novel())
	if err != nil {
		slog.Error("Configure subject plugins failed", "error", err)
		os.Exit(1)
	}

	// infra
	db := infra.NewSQLDB(
		env("DB_HOST", "localhost"),
		envInt("DB_PORT", 5001),
		env("DB_USER", "forum"),
		env("DB_PASSWORD", ""),
		env("DB_NAME", "forum"),
	)
	defer db.Close()

	// repository
	transactions := repository.NewTransactionManager(db)
	tagRepo := repository.NewTagRepository(db)
	postRepo := repository.NewPostRepository(db, tagRepo)
	commentRepo := repository.NewCommentRepository(db)
	favoriteRepo := repository.NewFavoriteRepository(db)

	postUsecase := usecase.NewPostUsecase(transactions, postRepo, tagRepo, favoriteRepo, domains)
	commentUsecase := usecase.NewCommentUsecase(transactions, commentRepo, postRepo, domains, subjects)
	tagUsecase := usecase.NewTagUsecase(tagRepo)

	// handler
	categoryHandler := handler.NewCategoryHandler(tagUsecase)
	postHandler := handler.NewPostHandler(postUsecase, commentUsecase)
	commentHandler := handler.NewCommentHandler(commentUsecase)
	externalCommentHandler := handler.NewExternalCommentHandler(commentUsecase)
	meHandler := handler.NewMeHandler(postUsecase)

	// router
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK\n"))
	})
	router.Route("/v1", func(router chi.Router) {
		router.Use(httplog.RequestLogger(slog.Default(), &httplog.Options{
			Level:         slog.LevelInfo,
			Schema:        httplog.SchemaECS,
			RecoverPanics: true,
		}))
		router.Use(httpx.OptionalAccessToken)
		router.Route("/category", categoryHandler.RegisterRoutes)
		router.Route("/post", postHandler.RegisterRoutes)
		router.Route("/comment", commentHandler.RegisterRoutes)
		router.Route("/external/comment", externalCommentHandler.RegisterRoutes)
		router.Route("/me", meHandler.RegisterRoutes)
		router.Route("/admin", func(router chi.Router) {
			router.Use(httpx.RequireAdmin)
			router.Route("/category", categoryHandler.RegisterAdminRoutes)
			router.Route("/post", postHandler.RegisterAdminRoutes)
			router.Route("/comment", commentHandler.RegisterAdminRoutes)
		})
	})

	// start server
	slog.Info("Listening on localhost:8080...")
	if err := http.ListenAndServe(":8080", router); err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}
