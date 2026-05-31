package storage

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"cloud.google.com/go/firestore"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/iterator"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ Storage = (*FirestoreStorage)(nil)

// FirestoreStorage implements Storage using Google Cloud Firestore as the backend.
type FirestoreStorage struct {
	client     *firestore.Client
	collection string
}

// NewFirestoreStorage creates a new FirestoreStorage with the given Firestore client and collection.
func NewFirestoreStorage(client *firestore.Client, collection string) *FirestoreStorage {
	return &FirestoreStorage{
		client:     client,
		collection: collection,
	}
}

func (s *FirestoreStorage) Get(ctx context.Context, path string) (*RepoConfig, error) {
	ctx, span := tracer.Start(ctx, "FirestoreStorage.Get")
	defer span.End()

	path = encodePath(path)
	doc, err := s.client.Collection(s.collection).Doc(path).Get(ctx)
	if status.Code(err) == grpccodes.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var config RepoConfig
	if err := doc.DataTo(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *FirestoreStorage) Set(ctx context.Context, path string, config *RepoConfig) error {
	ctx, span := tracer.Start(ctx, "FirestoreStorage.Set")
	defer span.End()

	path = encodePath(path)
	_, err := s.client.Collection(s.collection).Doc(path).Set(ctx, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		span.AddEvent("failed to set document", trace.WithAttributes(attribute.String("document.id", path)))
	}
	return err
}

func (s *FirestoreStorage) ListAll(ctx context.Context) ([]string, error) {
	ctx, span := tracer.Start(ctx, "FirestoreStorage.ListAll")
	defer span.End()

	var paths []string
	iter := s.client.Collection(s.collection).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		rp, err := decodePath(doc.Ref.ID)
		if err != nil {
			// TODO: log error
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.LogAttrs(ctx, slog.LevelError, "failed to decode URL base64", slog.String("encoded_path", doc.Ref.ID), slog.String("error", err.Error()))
		} else {
			paths = append(paths, rp)
		}
	}
	span.AddEvent("retrieve paths", trace.WithAttributes(attribute.StringSlice("retrieved_ids", paths)))
	return paths, nil
}

func (s *FirestoreStorage) Delete(ctx context.Context, path string) error {
	ctx, span := tracer.Start(ctx, "FirestoreStorage.Delete")
	defer span.End()

	path = encodePath(path)
	_, err := s.client.Collection(s.collection).Doc(path).Delete(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.AddEvent("failed to delete document", trace.WithAttributes(attribute.String("document.id", path)))
	}
	return err
}

func (s *FirestoreStorage) Close(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "FirestoreStorage.Close")
	defer span.End()
	return s.client.Close()
}

func encodePath(path string) string {
	return url.PathEscape(strings.TrimPrefix(path, "/"))
}

func decodePath(encodedPath string) (string, error) {
	p, err := url.PathUnescape(encodedPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p, nil
}
