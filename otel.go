package govanityurls

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
)

// slogLimitedLogger is a limited logger interface for slog.Logger
// It emphasizes the context of the process, and the severity of the logging record.
type slogLimitedLogger interface {
	LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
	WithGroup(name string) *slog.Logger
}

var tracer = otel.Tracer("github.com/markxp/govanityurls")
