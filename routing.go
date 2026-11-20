package govanityurls

import (
	"iter"
	"maps"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Middleware wraps an http.Handler with cross-cutting logic such as authentication.
type Middleware func(http.Handler) http.Handler

type handlerOptions struct {
	adminMiddleware    Middleware
	internalMiddleware Middleware
	otelOpts           []otelhttp.Option
}

// HandlerOption configures how RegisterHandlers mounts routes on an http.ServeMux.
type HandlerOption func(*handlerOptions)

// WithAdminMiddleware wraps admin routes (/_admin and /_admin/{path...}) with mw.
// It panics if mw is nil.
func WithAdminMiddleware(mw Middleware) HandlerOption {
	if mw == nil {
		panic("govanityurls: WithAdminMiddleware requires a non-nil Middleware")
	}
	return func(o *handlerOptions) {
		o.adminMiddleware = mw
	}
}

// WithInternalMiddleware wraps internal routes (e.g., /_internal/registerRepo) with mw.
// It panics if mw is nil.
func WithInternalMiddleware(mw Middleware) HandlerOption {
	if mw == nil {
		panic("govanityurls: WithInternalMiddleware requires a non-nil Middleware")
	}
	return func(o *handlerOptions) {
		o.internalMiddleware = mw
	}
}

// WithPrivateMiddleware wraps both admin and internal routes with mw.
// It panics if mw is nil.
func WithPrivateMiddleware(mw Middleware) HandlerOption {
	if mw == nil {
		panic("govanityurls: WithPrivateMiddleware requires a non-nil Middleware")
	}
	return func(o *handlerOptions) {
		o.adminMiddleware = mw
		o.internalMiddleware = mw
	}
}

// WithOTELOptions appends OpenTelemetry HTTP instrumentation options to every registered route.
func WithOTELOptions(opts ...otelhttp.Option) HandlerOption {
	return func(o *handlerOptions) {
		o.otelOpts = append(o.otelOpts, opts...)
	}
}

func (app *App) publicHandlers() iter.Seq2[string, http.Handler] {
	m := map[string]http.Handler{
		"GET /{$}":        http.HandlerFunc(app.getIndex),
		"GET /{path...}": http.HandlerFunc(app.getRepo),
	}
	return maps.All(m)
}

// adminHandlers returns the sequence of un-instrumented admin route patterns and leaf handlers.
func (app *App) adminHandlers() iter.Seq2[string, http.Handler] {
	m := map[string]http.Handler{
		"POST /_admin":          http.HandlerFunc(app.postRegisterRepoForm),
		"GET /_admin":           http.HandlerFunc(app.getRegisterRepoForm),
		"GET /_admin/{path...}": http.HandlerFunc(app.getRepoConfig),
	}
	return maps.All(m)
}

// internalHandlers returns the sequence of un-instrumented internal route patterns and leaf handlers.
func (app *App) internalHandlers() iter.Seq2[string, http.Handler] {
	m := make(map[string]http.Handler)
	if app.asyncCfg != nil {
		m["POST /_internal/registerRepo"] = http.HandlerFunc(app.asyncRegisterRepo)
	}
	return maps.All(m)
}

// RegisterHandlers mounts public, admin, and internal routes onto mux with OpenTelemetry
// instrumentation wrapping the outer layer (including any configured authentication middleware).
//
// If WithAdminMiddleware is provided without WithInternalMiddleware when async registration
// is enabled, RegisterHandlers panics to fail closed rather than exposing internal endpoints
// without authentication.
func (app *App) RegisterHandlers(mux *http.ServeMux, opts ...HandlerOption) {
	var cfg handlerOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if cfg.adminMiddleware != nil && cfg.internalMiddleware == nil && app.asyncCfg != nil {
		panic("govanityurls: RegisterHandlers requires WithInternalMiddleware when async registration and WithAdminMiddleware are configured")
	}

	noopMW := func(h http.Handler) http.Handler { return h }
	adminMW := cfg.adminMiddleware
	if adminMW == nil {
		adminMW = noopMW
	}
	internalMW := cfg.internalMiddleware
	if internalMW == nil {
		internalMW = noopMW
	}

	for pattern, h := range app.publicHandlers() {
		mux.Handle(pattern, instrumentRoute(pattern, h, cfg.otelOpts...))
	}
	for pattern, h := range app.adminHandlers() {
		mux.Handle(pattern, instrumentRoute(pattern, adminMW(h), cfg.otelOpts...))
	}
	for pattern, h := range app.internalHandlers() {
		mux.Handle(pattern, instrumentRoute(pattern, internalMW(h), cfg.otelOpts...))
	}
}

// GetPublicHandlers returns a sequence of OpenTelemetry-instrumented public handlers.
// The string key uses Go 1.22+ http.ServeMux pattern syntax.
//
// Deprecated: Use [App.RegisterHandlers] instead.
func (app *App) GetPublicHandlers() iter.Seq2[string, http.Handler] {
	return func(yield func(string, http.Handler) bool) {
		for pattern, h := range app.publicHandlers() {
			if !yield(pattern, instrumentRoute(pattern, h)) {
				return
			}
		}
	}
}

// GetPrivateHandlers returns a sequence of OpenTelemetry-instrumented private handlers
// (both admin and internal). The string key uses Go 1.22+ http.ServeMux pattern syntax.
//
// Deprecated: Use [App.RegisterHandlers] instead.
func (app *App) GetPrivateHandlers() iter.Seq2[string, http.Handler] {
	return func(yield func(string, http.Handler) bool) {
		for pattern, h := range app.adminHandlers() {
			if !yield(pattern, instrumentRoute(pattern, h)) {
				return
			}
		}
		for pattern, h := range app.internalHandlers() {
			if !yield(pattern, instrumentRoute(pattern, h)) {
				return
			}
		}
	}
}
