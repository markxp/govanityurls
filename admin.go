package govanityurls

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/markxp/govanityurls/storage"
	"go.opentelemetry.io/otel/trace"
)

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

type registerRepoPayload struct {
	Path       string              `json:"path"`
	RepoConfig *storage.RepoConfig `json:"repoConfig"`
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
	ctx := r.Context()

	path := strings.Trim(r.PathValue("path"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
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
		app.logger.LogAttrs(ctx, slog.LevelError, "Failed to render repo config", slog.String("path", path), slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// postRegisterRepoForm registers a new repository. It is for the users to hit.
// If the app has a task client, it will create a task to do the actual "register repo" job asynchronously.
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
	if !strings.HasPrefix(cr.Path, "/") {
		cr.Path = "/" + cr.Path
	}
	trace.SpanFromContext(r.Context()).SetAttributes(
		attrModulePath.String(strings.TrimPrefix(cr.Path, "/")),
	)
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

	// Fires a task to do the actual "create repo" job asynchronously.
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

// asyncRegisterRepo does the actual "register repo" job asynchronously, triggered by TaskSubmitter.
func (app *App) asyncRegisterRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// decode the json payload and write to the storage.
	var payload WriteConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		app.logger.LogAttrs(ctx, slog.LevelInfo, "Failed to decode payload", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if payload.Path == "" || payload.Config == nil {
		app.logger.LogAttrs(ctx, slog.LevelInfo, "Payload path or config is missing", slog.Any("payload", payload))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(payload.Path, "/") {
		payload.Path = "/" + payload.Path
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attrModulePath.String(strings.TrimPrefix(payload.Path, "/")),
	)

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

func (app *App) clearCache(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "clearCache")
	defer span.End()

	var memStore *storage.InMemoryCache
	var ok bool
	if memStore, ok = app.storage.(*storage.InMemoryCache); !ok {
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = io.WriteString(w, "cache cannot be cleared or cache is unavailable")
		return
	}

	memStore.Clear(ctx)
	w.WriteHeader(http.StatusNoContent)
}
