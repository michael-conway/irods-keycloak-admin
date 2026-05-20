package audit

import (
	"context"
	"log/slog"
)

type Event struct {
	RequestID string
	Actor     string
	Source    string
	Operation string
	Target    string
	Result    string
}

type Logger interface {
	Record(ctx context.Context, event Event)
}

type SlogLogger struct{}

func (SlogLogger) Record(ctx context.Context, event Event) {
	slog.InfoContext(
		ctx,
		"audit",
		"request_id", event.RequestID,
		"actor", event.Actor,
		"source", event.Source,
		"operation", event.Operation,
		"target", event.Target,
		"result", event.Result,
	)
}
