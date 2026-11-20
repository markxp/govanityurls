package govanityurls

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/markxp/govanityurls/contrib/github"
	"github.com/markxp/govanityurls/registry"
	"github.com/markxp/govanityurls/storage"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type recordingTracerProvider struct {
	noop.TracerProvider
	mu    sync.Mutex
	spans []*recordingSpan
}

func (tp *recordingTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &recordingTracer{tp: tp}
}

func (tp *recordingTracerProvider) recordedSpans() []*recordingSpan {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	out := make([]*recordingSpan, len(tp.spans))
	copy(out, tp.spans)
	return out
}

type recordingTracer struct {
	noop.Tracer
	tp *recordingTracerProvider
}

func (tr *recordingTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	cfg := trace.NewSpanStartConfig(opts...)
	attrs := make(map[attribute.Key]attribute.Value)
	for _, kv := range cfg.Attributes() {
		attrs[kv.Key] = kv.Value
	}

	tr.tp.mu.Lock()
	idx := byte(len(tr.tp.spans) + 1)
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{idx},
		TraceFlags: trace.FlagsSampled,
	})
	sp := &recordingSpan{
		tp:    tr.tp,
		name:  spanName,
		sc:    sc,
		attrs: attrs,
	}
	tr.tp.spans = append(tr.tp.spans, sp)
	tr.tp.mu.Unlock()

	return trace.ContextWithSpan(ctx, sp), sp
}

type recordingSpan struct {
	noop.Span
	tp    *recordingTracerProvider
	sc    trace.SpanContext
	mu    sync.Mutex
	name  string
	attrs map[attribute.Key]attribute.Value
	ended bool
}

func (s *recordingSpan) SpanContext() trace.SpanContext       { return s.sc }
func (s *recordingSpan) IsRecording() bool                    { return true }
func (s *recordingSpan) TracerProvider() trace.TracerProvider { return s.tp }

func (s *recordingSpan) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range kv {
		s.attrs[item.Key] = item.Value
	}
}

func (s *recordingSpan) End(opts ...trace.SpanEndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

func (s *recordingSpan) snapshot() (string, map[attribute.Key]attribute.Value, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[attribute.Key]attribute.Value, len(s.attrs))
	maps.Copy(cp, s.attrs)
	return s.name, cp, s.ended
}

func TestOTELHandlerPropagation(t *testing.T) {
	t.Run("GET /{path...} enriches server span with module and subpath attributes", func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{
			"/a/b": {Repo: "https://github.com/user/b", VCS: "git", Subdir: "b"},
		}}
		tp := &recordingTracerProvider{}
		app := NewApp("example.com", 300, store, nil, []registry.Registry{&github.GithubRegistry{}}, nil, nil)
		mux := http.NewServeMux()
		app.RegisterHandlers(mux, WithOTELOptions(otelhttp.WithTracerProvider(tp)))

		req := httptest.NewRequest(http.MethodGet, "/a/b/c?go-get=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		spans := tp.recordedSpans()
		if len(spans) != 1 {
			t.Fatalf("recorded %d spans, want 1", len(spans))
		}
		name, attrs, ended := spans[0].snapshot()
		if !ended {
			t.Errorf("expected server span to be ended")
		}
		if name != "GET /{path...}" {
			t.Errorf("span name = %q, want %q", name, "GET /{path...}")
		}
		if got := attrs[attrRequestPath].AsString(); got != "a/b/c" {
			t.Errorf("govanityurls.request.path = %q, want %q", got, "a/b/c")
		}
		if got := attrs[attrModulePath].AsString(); got != "a/b" {
			t.Errorf("govanityurls.module.path = %q, want %q", got, "a/b")
		}
		if got := attrs[attrModuleSubpath].AsString(); got != "c" {
			t.Errorf("govanityurls.module.subpath = %q, want %q", got, "c")
		}
		if got := attrs[attrGoGet].AsBool(); !got {
			t.Errorf("govanityurls.go_get = %v, want true", got)
		}
	})

	t.Run("GET /_admin/{path...} traces 401 middleware rejection with request path", func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{}}
		tp := &recordingTracerProvider{}
		app := NewApp("example.com", 300, store, nil, nil, nil, nil)

		var middlewareSawValidSpan bool
		rejectAdminMW := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareSawValidSpan = trace.SpanFromContext(r.Context()).SpanContext().IsValid()
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			})
		}

		mux := http.NewServeMux()
		app.RegisterHandlers(
			mux,
			WithAdminMiddleware(rejectAdminMW),
			WithOTELOptions(otelhttp.WithTracerProvider(tp)),
		)

		req := httptest.NewRequest(http.MethodGet, "/_admin/my-repo/sub", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if !middlewareSawValidSpan {
			t.Errorf("expected auth middleware to execute inside an active OTEL span")
		}

		spans := tp.recordedSpans()
		if len(spans) != 1 {
			t.Fatalf("recorded %d spans, want 1", len(spans))
		}
		name, attrs, ended := spans[0].snapshot()
		if !ended {
			t.Errorf("expected server span to be ended")
		}
		if name != "GET /_admin/{path...}" {
			t.Errorf("span name = %q, want %q", name, "GET /_admin/{path...}")
		}
		if got := attrs[attrRequestPath].AsString(); got != "my-repo/sub" {
			t.Errorf("govanityurls.request.path = %q, want %q", got, "my-repo/sub")
		}
	})

	t.Run("POST /_admin and POST /_internal/registerRepo set govanityurls.module.path", func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{}}
		tp := &recordingTracerProvider{}
		asyncCfg := &AsyncRegisterConfig{TaskClient: nil}
		app := NewApp("example.com", 300, store, nil, []registry.Registry{&github.GithubRegistry{}}, asyncCfg, nil)

		mux := http.NewServeMux()
		app.RegisterHandlers(mux, WithOTELOptions(otelhttp.WithTracerProvider(tp)))

		form := strings.NewReader("path=foo/bar&repo=https://github.com/example/bar&vcs=git")
		reqAdmin := httptest.NewRequest(http.MethodPost, "/_admin", form)
		reqAdmin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recAdmin := httptest.NewRecorder()
		mux.ServeHTTP(recAdmin, reqAdmin)
		if recAdmin.Code != http.StatusOK {
			t.Fatalf("POST /_admin status = %d, want 200", recAdmin.Code)
		}

		body := `{"path":"/baz/qux","config":{"repo":"https://github.com/example/qux","vcs":"git"}}`
		reqInternal := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(body))
		recInternal := httptest.NewRecorder()
		mux.ServeHTTP(recInternal, reqInternal)
		if recInternal.Code != http.StatusCreated {
			t.Fatalf("POST /_internal/registerRepo status = %d, want 201", recInternal.Code)
		}

		spans := tp.recordedSpans()
		if len(spans) != 2 {
			t.Fatalf("recorded %d server spans, want 2", len(spans))
		}
		name0, attrs0, _ := spans[0].snapshot()
		if name0 != "POST /_admin" || attrs0[attrModulePath].AsString() != "foo/bar" {
			t.Errorf("span[0] = (%q, %v), want POST /_admin with module.path=foo/bar", name0, attrs0)
		}
		name1, attrs1, _ := spans[1].snapshot()
		if name1 != "POST /_internal/registerRepo" || attrs1[attrModulePath].AsString() != "baz/qux" {
			t.Errorf("span[1] = (%q, %v), want POST /_internal/registerRepo with module.path=baz/qux", name1, attrs1)
		}
	})
}
