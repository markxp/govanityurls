package govanityurls

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// slogLimitedLogger is a limited logger interface for slog.Logger
// It emphasizes the context of the process, and the severity of the logging record.
type slogLimitedLogger interface {
	LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
	WithGroup(name string) *slog.Logger
}

var tracer = otel.Tracer("github.com/markxp/govanityurls")

const (
	attrRequestPath   = attribute.Key("govanityurls.request.path")
	attrModulePath    = attribute.Key("govanityurls.module.path")
	attrModuleSubpath = attribute.Key("govanityurls.module.subpath")
	attrGoGet         = attribute.Key("govanityurls.go_get")
)

func instrumentRoute(pattern string, handler http.Handler, opts ...otelhttp.Option) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqPath := strings.Trim(r.PathValue("path"), "/"); reqPath != "" {
			trace.SpanFromContext(r.Context()).SetAttributes(
				attrRequestPath.String(reqPath),
			)
		}
		handler.ServeHTTP(w, r)
	})
	return otelhttp.NewHandler(inner, pattern, opts...)
}
