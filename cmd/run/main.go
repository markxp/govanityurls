package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/markxp/govanityurls"
	oo "github.com/markxp/govanityurls/auth/oauth2"
	"github.com/markxp/govanityurls/auth/oauth2/google"
	"github.com/markxp/govanityurls/storage"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
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

	clientID := os.Getenv("OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH_CLIENT_SECRET")
	redirectURL := os.Getenv("OAUTH_REDIRECT_URL")

	if clientID == "" {
		slog.Error("OAUTH_CLIENT_ID environment variable is not set")
		os.Exit(1)
	}

	// auth: we use OIDC

	// 1. prepare standard oauth2.Config
	var oauthConfig *oauth2.Config = google.OAuth2Config(clientID, clientSecret, redirectURL)

	// 2. prepare the allow list for emails and domains (with limited whitelists)
	var allowedEmails []string
	if emailsStr := os.Getenv("ALLOWED_EMAILS"); emailsStr != "" {
		for _, e := range strings.Split(emailsStr, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				allowedEmails = append(allowedEmails, trimmed)
			}
		}
	}

	var allowedDomains []string
	if domainsStr := os.Getenv("ALLOWED_DOMAINS"); domainsStr != "" {
		for _, d := range strings.Split(domainsStr, ",") {
			if trimmed := strings.TrimSpace(d); trimmed != "" {
				allowedDomains = append(allowedDomains, trimmed)
			}
		}
	}

	// 3. prepare the magic secret for state signing
	stateSecretStr := os.Getenv("OAUTH_STATE_SECRET")
	var stateSecret []byte
	if stateSecretStr != "" {
		stateSecret = []byte(stateSecretStr)
	} else {
		slog.Warn("OAUTH_STATE_SECRET is not set. Generating a random key for OAuth state signatures. Note that state verification will fail across restarts or multi-instance deployments.")
		stateSecret = make([]byte, 32)
		if _, err := rand.Read(stateSecret); err != nil {
			slog.Error("Failed to generate random state secret", slog.Any("error", err))
			os.Exit(1)
		}
	}

	// 4. Create a single auth middleware
	// 4.1 prepare the options for the auth middleware
	opts := []oo.Option{
		oo.WithAllowedEmails(allowedEmails),
		oo.WithAllowedDomains(allowedDomains),
		oo.WithLogger(logger),
	}

	// 4.2 prepare the auth middleware with `login` path, `defaultLandingPath` path for staring and ending of the oauth2 flow.
	authMiddleware, loginHandler, callbackHandler := oo.New(oauthConfig, stateSecret, "/_admin/login", "/_admin", opts...)

	app := govanityurls.NewApp(vanityHost, cacheMaxAgeSec, store, nil, nil, nil, logger)

	// use these handlers, middlewares as is.
	mux := http.NewServeMux()

	// Register public handlers
	for path, handler := range app.GetPublicHandlers() {
		mux.Handle(path, handler)
	}

	// Mount login and callback handlers explicitly on the mux
	mux.Handle("GET /_admin/login", loginHandler)
	mux.Handle("GET /_admin/callback", callbackHandler)

	// Register private handlers on the mux, wrapping them with the auth middleware
	for path, handler := range app.GetPrivateHandlers() {
		mux.Handle(path, authMiddleware(handler))
	}

	handler := mux

	healthPort := os.Getenv("HEALTH_PORT")
	if healthPort == "" {
		healthPort = "8081"
	}

	healthServer := &http.Server{
		Addr: ":" + healthPort,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}),
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	// Register App's storage shutdown handler
	app.RegisterShutdownFunc(server)

	// Graceful shutdown signaling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Starting health check server on Cloud Run", slog.String("port", healthPort))
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start health check server", slog.Any("error", err))
		}
	}()

	go func() {
		slog.Info("Starting HTTP server on Cloud Run", slog.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start server", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-stop
	slog.Info("Shutting down server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Failed to gracefully shutdown health check server", slog.Any("error", err))
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Failed to gracefully shutdown server", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("Server stopped")
}
