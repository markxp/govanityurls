package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/markxp/govanityurls"
	"github.com/markxp/govanityurls/auth/iap"
	"github.com/markxp/govanityurls/storage"
	"go.opentelemetry.io/otel/trace"
)

// gcpTraceHandler wraps a slog.Handler to inject GCP-compatible trace and span IDs.
type gcpTraceHandler struct {
	slog.Handler
	projectID string
}

func (h *gcpTraceHandler) Handle(ctx context.Context, r slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		if h.projectID != "" {
			r.AddAttrs(slog.String("logging.googleapis.com/trace", fmt.Sprintf("projects/%s/traces/%s", h.projectID, spanContext.TraceID().String())))
		} else {
			r.AddAttrs(slog.String("logging.googleapis.com/trace", spanContext.TraceID().String()))
		}
		r.AddAttrs(slog.String("logging.googleapis.com/spanId", spanContext.SpanID().String()))
		r.AddAttrs(slog.Bool("logging.googleapis.com/trace_sampled", spanContext.IsSampled()))
	}
	return h.Handler.Handle(ctx, r)
}


func main() {
	// Initialize GCP trace-aware JSON logger.
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(&gcpTraceHandler{
		Handler:   jsonHandler,
		projectID: projectID,
	})
	slog.SetDefault(logger)

	if projectID == "" {
		slog.Error("GOOGLE_CLOUD_PROJECT or PROJECT_ID environment variable is not set")
		os.Exit(1)
	}

	collection := os.Getenv("FIRESTORE_COLLECTION")
	if collection == "" {
		collection = "vanity_urls"
	}

	ctx := context.Background()
	firestoreClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		slog.Error("Failed to initialize Firestore client", slog.Any("error", err))
		os.Exit(1)
	}

	store := storage.NewFirestoreStorage(firestoreClient, collection)

	vanityHost := os.Getenv("VANITY_HOST")
	cacheMaxAgeSec := 300
	if maxAgeStr := os.Getenv("CACHE_MAX_AGE"); maxAgeStr != "" {
		if val, err := strconv.Atoi(maxAgeStr); err == nil {
			cacheMaxAgeSec = val
		}
	}

	// auth: we use IAP

	iapAudience := os.Getenv("IAP_AUDIENCE")
	if iapAudience == "" {
		slog.Warn("IAP_AUDIENCE environment variable is not set; all IAP-protected requests will fail validation")
	}

	app := govanityurls.NewApp(vanityHost, cacheMaxAgeSec, store, nil, nil, nil, logger)

	mux := http.NewServeMux()

	// Register public handlers
	for path, handler := range app.GetPublicHandlers() {
		mux.Handle(path, handler)
	}

	// Register private handlers, wrapped in IAP validation middleware.
	iapValidator := iap.NewIAP(iapAudience, logger)
	iapMiddleware := iapValidator.Middleware

	for path, handler := range app.GetPrivateHandlers() {
		mux.Handle(path, iapMiddleware(handler))
	}

	mux.HandleFunc("GET /_ah/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	// Register App's storage shutdown handler.
	app.RegisterShutdownFunc(server)

	// Graceful shutdown signaling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Starting HTTP server", slog.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start server", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-stop
	slog.Info("Shutting down server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Failed to gracefully shutdown server", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("Server stopped")
}
