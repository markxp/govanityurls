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

package govanityurls

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/markxp/govanityurls/registry"
	"github.com/markxp/govanityurls/storage"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// FallbackConfig configures a fallback repository check registry.
type FallbackConfig struct {
	RepoChecker registry.RepoChecker
	Registry    registry.Registry
}

// AsyncRegisterConfig configures the task client and service account for asynchronous repository registration.
type AsyncRegisterConfig struct {
	TaskClient               TaskSubmitter
	TaskClientServiceAccount string
}

// App represents the main application instance.
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

// NewApp creates a new App which responds to Go's import meta tags.
// The host parameter specifies the host name to use in the import meta tags.
// The cacheMaxAge parameter specifies the amount of time in seconds to cache package pages.
// The store parameter is the storage backend used to retrieve and store repository configurations.
// The fallback parameter specifies an optional fallback registry checker.
// The registries parameter is a slice of registries used to fix repository configurations.
// The asyncCfg parameter specifies optional configuration for asynchronous registration.
// The logger parameter specifies the slog limited logger to use.
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

	return app
}

// RegisterShutdownFunc registers the shutdown operations (e.g., closing storage) to the HTTP server.
func (app *App) RegisterShutdownFunc(s *http.Server) {
	s.RegisterOnShutdown(func() {
		if err := app.storage.Close(context.TODO()); err != nil {
			app.logger.LogAttrs(context.TODO(), slog.LevelError, "Failed to close storage", slog.Any("error", err))
		}
		if app.asyncCfg != nil && app.asyncCfg.TaskClient != nil {
			if err := app.asyncCfg.TaskClient.Close(context.TODO()); err != nil {
				app.logger.LogAttrs(context.TODO(), slog.LevelError, "Failed to close task client", slog.Any("error", err))
			}
		}
	})
}

func (app *App) getIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	host := app.derivedHost(r)
	paths, err := app.storage.ListAll(ctx)
	if err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "Failed to list paths", slog.Any("error", err))
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

// getRepo returns an HTML web page to a human or the `go-get` tool.
//
// The strategy for finding a Go module is digging upward recursively to find a `go.mod` file.
// If a request has a URL path = `/a/b/c`, it will:
//
//  1. Try to find `a` as the repo name and also the root of `go.mod`, `/b/c` as the sub-package path in the same module.
//
// In this scenario, /a/b/c should return the `go-import` tag pointing to `example.com/a` as a Go module, the VCS, and the address of the source code repository.
func (app *App) getRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rawPath := strings.Trim(r.PathValue("path"), "/")
	if rawPath == "" {
		http.NotFound(w, r)
		return
	}

	isGoGet := r.URL.Query().Get("go-get") == "1"
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrGoGet.Bool(isGoGet))

	fullPathSegments := strings.Split(rawPath, "/")
	repoName := fullPathSegments[0]

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

	attrs := []attribute.KeyValue{
		attrModulePath.String(modulePath),
	}
	if subpath != "" {
		attrs = append(attrs, attrModuleSubpath.String(subpath))
	}
	span.SetAttributes(attrs...)

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
		IsGoGet: isGoGet,
	}); err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "Failed to render vanity page", slog.Any("error", err))
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
