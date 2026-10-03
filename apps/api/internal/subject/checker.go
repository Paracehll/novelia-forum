// Package subject adapts comment subject keys to each site's resource APIs.
package subject

import (
	"auth/internal/repository"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrNotFound    = errors.New("subject not found")
	ErrInvalid     = errors.New("invalid subject key")
	ErrUnsupported = errors.New("unsupported subject kind")
)

type Checker interface {
	Check(ctx context.Context, kind, key string) (int16, error)
}

type resourceType struct {
	id      int16
	resolve func(string) (string, error)
}

// IDs are persisted in comments and must never be reassigned.
var registry = map[string]resourceType{
	"novel": {id: repository.CommentSubjectNovel, resolve: novelResolver},
}

func TypeID(kind string) (int16, bool) {
	resource, ok := registry[kind]
	return resource.id, ok
}

type HTTPChecker struct {
	registry map[string]resourceType
	client   *http.Client
}

// NewHTTPChecker uses the built-in list of supported resource services.
// Resource APIs return 204 for existing resources, 404 for absent ones,
// 400 invalid key. All other responses fail closed, including redirects.
func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{
		registry: registry,
		client: &http.Client{
			Timeout:       3 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *HTTPChecker) Check(ctx context.Context, kind, key string) (int16, error) {
	resource, ok := c.registry[kind]
	if !ok {
		return 0, ErrUnsupported
	}
	endpoint, err := resource.resolve(key)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build subject request: %w", err)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("check subject: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNoContent:
		return resource.id, nil
	case http.StatusNotFound:
		return 0, ErrNotFound
	case http.StatusBadRequest:
		return 0, ErrInvalid
	default:
		return 0, fmt.Errorf("subject service returned HTTP %d", response.StatusCode)
	}
}

// ValidateKey checks a resource type and key without contacting the resource service.
func ValidateKey(kind, key string) error {
	resource, ok := registry[kind]
	if !ok {
		return ErrUnsupported
	}
	_, err := resource.resolve(key)
	return err
}
