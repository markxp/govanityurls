// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// govanityurls serves Go vanity URLs.
package govanityurls

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"encoding/json"

	"github.com/markxp/govanityurls/registry"
	"github.com/markxp/govanityurls/storage"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type FallbackConfig struct {
	RepoChecker registry.RepoChecker
	Registry    registry.Registry
}

type AsyncRegisterConfig struct {
	TaskClient               TaskSubmitter
	TaskClientServiceAccount string
}

type App struct {
	host        string
	cacheMaxAge int
	storage     storage.Storage

	// registries
	fallback *FallbackConfig
	fixer    registry.RepoConfigFixer

	// optional tools
	asyncCfg *AsyncRegisterConfig
	logger   slogLimitedLogger
}

// NewServer creates a new http.Server which responds to Go's import meta tags.
// The `addr` is http.Server.Addr. It is required, and the format "host:port" is not recommended here. please just use ":port".
// The `host` is the host name that will be used in the import meta tags. This is required if your app sets behind a reverse proxy, or environment like App Engine or Cloud Run but you only allows the source code to be retrived through the canonical custom domain URL.
// Besides, you can modify the timeouts of the server, just like a normal `http.Server`.
func NewApp(host string, cacheMaxAge int, store storage.Storage, fallback *FallbackConfig, registries []registry.Registry, asyncCfg *AsyncRegisterConfig, logger slogLimitedLogger) *App {

	// filter in default values for default config
	if cacheMaxAge <= 0 {
		cacheMaxAge = 300 // Default 5 minutes
	}

	app := &App{
		host:        host,
		cacheMaxAge: cacheMaxAge,
		storage:     store,
		fixer:       make(registry.RepoConfigFixer),
	}

	if fallback != nil {
		app.fallback = fallback
		app.fixer.Add(fallback.Registry)
	}
	for _, r := range registries {
		app.fixer.Add(r)
	}

	if logger == nil {
		logger = slog.Default()
	}
	app.logger = logger

	if asyncCfg != nil {
		app.asyncCfg = asyncCfg
	}

	// func() {
	// 	if err := app.storage.Close(); err != nil {
	// 		app.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to close storage", slog.Any("error", err))
	// 	}
	// 	if app.asyncCfg != nil {
	// 		if cloudTasksClient, ok := app.asyncCfg.TaskClient.(*CloudTasksSubmitter); ok {
	// 			if err := cloudTasksClient.Close(context.TODO()); err != nil {
	// 				app.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to close task client", slog.Any("error", err))
	// 			}
	// 		}
	// 	}
	// }()
	return app
}

// GetPublicHandlers returns a sequence of public handlers. It returns a string: http.Handler pair. The string is the path of the handler, which has the compatiable format for http.ServeMux (Go1.22+).
func (app *App) GetPublicHandlers() iter.Seq2[string, http.Handler] {
	m := make(map[string]http.Handler)
	m["GET /{$}"] = otelhttp.NewHandler(http.HandlerFunc(app.getIndex), "GET /{$}")
	m["GET /{repoName}"] = otelhttp.NewHandler(http.HandlerFunc(app.getRepo), "GET /{repoName}")
	m["GET /{repoName}/{path...}"] = otelhttp.NewHandler(http.HandlerFunc(app.getRepo), "GET /{repoName}/{path...}")
	return maps.All(m)
}

// GetPrivateHandlers returns a sequence of private handlers. It returns a string: http.Handler pair. The string is the path of the handler, which has the compatiable format for http.ServeMux (Go1.22+).
func (app *App) GetPrivateHandlers() iter.Seq2[string, http.Handler] {
	m := make(map[string]http.Handler)
	if app.asyncCfg != nil {
		m["POST /_internal/registerRepo"] = otelhttp.NewHandler(http.HandlerFunc(app.asyncRegisterRepo), "POST /_internal/registerRepo")
	}
	m["POST /_admin"] = otelhttp.NewHandler(http.HandlerFunc(app.postRegisterRepoForm), "POST /_admin")
	m["GET /_admin"] = otelhttp.NewHandler(http.HandlerFunc(app.getRegisterRepoForm), "GET /_admin")
	m["GET /_admin/{repoName}"] = otelhttp.NewHandler(http.HandlerFunc(app.getRepoConfig), "GET /_admin/{repoName}")
	m["GET /_admin/{repoName}/{path...}"] = otelhttp.NewHandler(http.HandlerFunc(app.getRepoConfig), "GET /_admin/{repoName}/{path...}")
	return maps.All(m)
}

func (app *App) RegisterShutdownFunc(s *http.Server) {
	s.RegisterOnShutdown(func() {
		if err := app.storage.Close(context.TODO()); err != nil {
			app.logger.LogAttrs(context.TODO(), slog.LevelError, "Failed to close storage", slog.Any("error", err))
		}
		if app.asyncCfg != nil {
			asyncClient := app.asyncCfg.TaskClient
			if err := asyncClient.Close(context.TODO()); err != nil {
				app.logger.LogAttrs(context.TODO(), slog.LevelError, "Failed to close task client", slog.Any("error", err))
			}
		}
	})
}

// getRepo returns HTML web page to human or `go-get` tool.
//
// The strategy of find a Go module is digging upward recursively to find a `go.mod` file.
// If a request has a URL path = `/a/b/c`, it will ...
// 1. Try to find `a` as repo name and also the root of `go.mod`, `/b/c` as the sub-package path in the same module.
// In this scenerio, /a/b/c should return the `go-import` tag points to `example.com/a` as a Go module, the VCS, the address of source code repository,
func (app *App) getRepo(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "getRepo")
	defer span.End()
	r = r.WithContext(ctx)

	repoName := r.PathValue("repoName")
	remainingPath := r.PathValue("path")

	// Construct full path segments
	var fullPathSegments []string
	if repoName != "" {
		fullPathSegments = append(fullPathSegments, repoName)
	}
	if remainingPath != "" {
		fullPathSegments = append(fullPathSegments, strings.Split(remainingPath, "/")...)
	}

	var config *storage.RepoConfig
	var err error
	var modulePath string
	var subpath string

	// 1. Recursive search for the module root (longest match first)
	for i := len(fullPathSegments); i > 0; i-- {
		currentPrefix := strings.Join(fullPathSegments[:i], "/")
		config, err = app.storage.Get(ctx, "/"+currentPrefix)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if config != nil {
			modulePath = currentPrefix
			if i < len(fullPathSegments) {
				subpath = strings.Join(fullPathSegments[i:], "/")
			}
			break
		}
	}

	// 2. Fallback to CSR for the first segment if no match found in storage
	// only support single segment repo name (no sub-modules, paths are for sub-packages, if any).
	if config == nil && app.fallback != nil {
		repoURL, err := app.fallback.RepoChecker.CheckRepo(ctx, repoName)
		if err == nil && repoURL != "" {
			config = &storage.RepoConfig{
				Repo: repoURL,
			}
			config, err = app.fixer.Fix(repoName, config)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			// Write back to storage
			_ = app.doRegisterRepo(ctx, repoName, config)
			modulePath = repoName
			if len(fullPathSegments) > 1 {
				subpath = strings.Join(fullPathSegments[1:], "/")
			}
		}
	}

	if config == nil {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", app.cacheMaxAge))
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", app.cacheMaxAge))
	if err := goVanityMetaTagTmpl.Execute(w, struct {
		Import  string
		Subpath string
		Repo    string
		Display string
		VCS     string
		Subdir  string
		IsGoGet bool
	}{
		Import:  app.derivedHost(r) + "/" + modulePath,
		Subpath: subpath,
		Repo:    config.Repo,
		Display: config.Display,
		VCS:     config.VCS,
		Subdir:  config.Subdir,
		IsGoGet: r.URL.Query().Get("go-get") == "1",
	}); err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "Failed to render vanity page", slog.Any("error", err))
		http.Error(w, "cannot render the page", http.StatusInternalServerError)
	}
}

// registerRepoFormData is the data structure for the admin form to register a new repository.
type registerRepoFormData struct {
	Host    string
	Message string
	Path    string
	Repo    string
	VCS     string
	Display string
	Subdir  string
}

// getRegisterRepoForm renders the admin form to register a new repository.
func (app *App) getRegisterRepoForm(w http.ResponseWriter, r *http.Request) {
	if err := registerRepoTmpl.Execute(w, &registerRepoFormData{Host: app.derivedHost(r)}); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to render: GET /_admin", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// getRepoConfig renders the admin form with pre-filled values for an existing repository.
func (app *App) getRepoConfig(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "getRepoConfig")
	defer span.End()
	r = r.WithContext(ctx)

	repoName := r.PathValue("repoName")
	remainingPath := r.PathValue("path")

	path := repoName
	if remainingPath != "" {
		path = repoName + "/" + remainingPath
	}

	config, err := app.storage.Get(ctx, "/"+path)
	if err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "Failed to get repo config", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if config == nil {
		http.NotFound(w, r)
		return
	}

	data := &registerRepoFormData{
		Host:    app.derivedHost(r),
		Path:    path,
		Repo:    config.Repo,
		VCS:     config.VCS,
		Display: config.Display,
		Subdir:  config.Subdir,
	}

	if err := registerRepoTmpl.Execute(w, data); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to render: GET /_admin/"+path, slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// postRegisterRepoForm registers a new repository. It is for the users to hit.
// If the app has a task client, it will create a task to do the actual "register repo" job asynchrously.
// Otherwise, it will do the job synchronously.
func (app *App) postRegisterRepoForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "cannot parse form:"+err.Error(), http.StatusBadRequest)
		return
	}
	cr := &registerRepoPayload{
		Path: r.FormValue("path"),
		RepoConfig: &storage.RepoConfig{
			Repo:    r.FormValue("repo"),
			VCS:     r.FormValue("vcs"),
			Display: r.FormValue("display"),
			Subdir:  r.FormValue("subdir"),
		},
	}

	if cr.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	if _, err := app.fixer.Fix(cr.Path, cr.RepoConfig); err != nil {
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	if err := app.doRegisterRepo(r.Context(), cr.Path, cr.RepoConfig); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to create repo", slog.Any("error", err))
		http.Error(w, "failed to register the Go module, that's all we know.", http.StatusInternalServerError)
		return
	}

	data := &registerRepoFormData{
		Host:    app.derivedHost(r),
		Message: "The Go module is registered successfully!",
	}
	if err := registerRepoTmpl.Execute(w, data); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to render admin form", slog.Any("error", err))
	}
}

func (app *App) doRegisterRepo(ctx context.Context, path string, rc *storage.RepoConfig) error {
	ctx, span := tracer.Start(ctx, "doRegisterRepo")
	defer span.End()

	if app.asyncCfg == nil || app.asyncCfg.TaskClient == nil {
		return app.storage.Set(ctx, path, rc)
	}

	// Fires a task to do the actual "create repo" job asynchrously.
	taskPayload := &WriteConfigPayload{
		Path:   path,
		Config: rc,
	}

	err := app.asyncCfg.TaskClient.CreateTask(ctx, taskPayload)
	if err == nil {
		return nil
	}

	// If task creation fails, log and fallback to sync write.
	app.logger.LogAttrs(ctx, slog.LevelError, "Failed to create task", slog.Any("error", err))
	return app.storage.Set(ctx, path, rc)
}

type registerRepoPayload struct {
	Path       string              `json:"path"`
	RepoConfig *storage.RepoConfig `json:"repoConfig"`
}

// asyncRegisterRepo does the actual "register repo" job asynchrously, triggered by TaskSubmitter.
func (app *App) asyncRegisterRepo(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "asyncRegisterRepo")
	defer span.End()
	r = r.WithContext(ctx)
	// decode the json payload and write to the storage.
	var payload WriteConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelInfo, "Failed to decode payload", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if payload.Path == "" || payload.Config == nil {
		app.logger.LogAttrs(r.Context(), slog.LevelInfo, "Payload path or config is missing", slog.Any("payload", payload))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if _, err := app.fixer.Fix(payload.Path, payload.Config); err != nil {
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	if err := app.storage.Set(ctx, payload.Path, payload.Config); err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "Failed to write config to storage", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (app *App) getIndex(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "getIndex")
	defer span.End()
	r = r.WithContext(ctx)

	host := app.derivedHost(r)
	paths, err := app.storage.ListAll(r.Context())
	if err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to list paths", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if err := indexTmpl.Execute(w, struct {
		Host  string
		Paths []string
	}{
		Host:  host,
		Paths: paths,
	}); err != nil {
		http.Error(w, "cannot render the page", http.StatusInternalServerError)
	}
}

func (app *App) derivedHost(r *http.Request) string {
	host := app.host
	if host == "" {
		host = r.Host
	}
	return host
}

func (app *App) clearCache(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "clearCache")
	defer span.End()
	// No need to propagate context here as it just calls Clear() which doesn't take context (or we don't pass it)
	// Actually Clear() is on *InMemoryCache, let's see.

	var memStore *storage.InMemoryCache
	var ok bool
	if memStore, ok = app.storage.(*storage.InMemoryCache); !ok {
		w.WriteHeader(http.StatusNotImplemented)
		io.WriteString(w, "cache cannot be cleared or cache is unavailable")
		return
	}

	memStore.Clear(ctx)
	w.WriteHeader(http.StatusNoContent)
}
