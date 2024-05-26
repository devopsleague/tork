package service

import (
	"context"

	"github.com/runabol/tork"
)

type EventType string

const (
	// StateChange occurs when a service's state changes.
	// Handler can inspect the service's State property
	// in order to determine what state the service is at.
	StateChange EventType = "STATE_CHANGE"
)

type HandlerFunc func(ctx context.Context, et EventType, j *tork.Service) error

func NoOpHandlerFunc(ctx context.Context, et EventType, j *tork.Service) error { return nil }

type MiddlewareFunc func(next HandlerFunc) HandlerFunc

func ApplyMiddleware(h HandlerFunc, mws []MiddlewareFunc) HandlerFunc {
	return func(ctx context.Context, et EventType, s *tork.Service) error {
		nx := next(ctx, 0, mws, h)
		return nx(ctx, et, s)
	}
}

func next(ctx context.Context, index int, mws []MiddlewareFunc, h HandlerFunc) HandlerFunc {
	if index >= len(mws) {
		return h
	}
	return mws[index](next(ctx, index+1, mws, h))
}
