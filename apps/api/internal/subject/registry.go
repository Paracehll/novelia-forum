// Package subject provides pluggable adapters for external comment subjects.
package subject

import (
	"context"
	"errors"
	"fmt"

	"forum/internal/domain"
)

type CheckFunc func(context.Context, string) bool

type Plugin struct {
	Kind        string
	SubjectType domain.CommentSubjectType
	Validate    func(string) bool
	Check       CheckFunc
}

type Registry struct {
	plugins map[string]Plugin
}

func NewRegistry(plugins ...Plugin) (*Registry, error) {
	registry := &Registry{plugins: make(map[string]Plugin, len(plugins))}
	types := make(map[domain.CommentSubjectType]string, len(plugins))
	for _, plugin := range plugins {
		if plugin.Kind == "" || plugin.SubjectType <= domain.CommentSubjectPost || plugin.Validate == nil || plugin.Check == nil {
			return nil, errors.New("invalid subject plugin")
		}
		if _, exists := registry.plugins[plugin.Kind]; exists {
			return nil, fmt.Errorf("duplicate subject kind %q", plugin.Kind)
		}
		if kind, exists := types[plugin.SubjectType]; exists {
			return nil, fmt.Errorf("subject type %d is already used by %q", plugin.SubjectType, kind)
		}
		registry.plugins[plugin.Kind] = plugin
		types[plugin.SubjectType] = plugin.Kind
	}
	return registry, nil
}

func (r *Registry) Type(kind string) (domain.CommentSubjectType, bool) {
	if r == nil {
		return 0, false
	}
	plugin, ok := r.plugins[kind]
	return plugin.SubjectType, ok
}

func (r *Registry) Valid(kind, key string) bool {
	if r == nil {
		return false
	}
	plugin, ok := r.plugins[kind]
	return ok && plugin.Validate(key)
}

func (r *Registry) Check(ctx context.Context, kind, key string) bool {
	if r == nil {
		return false
	}
	plugin, ok := r.plugins[kind]
	return ok && plugin.Check(ctx, key)
}
