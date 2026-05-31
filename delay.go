package govanityurls

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/markxp/govanityurls/storage"
)

// WriteConfigPayload defines the payload for creating a repo configuration.
type WriteConfigPayload struct {
	Path   string              `json:"path"`
	Config *storage.RepoConfig `json:"config"`
}

// TaskSubmitter allows asynchronous write-back to storage.
type TaskSubmitter interface {
	CreateTask(ctx context.Context, payload *WriteConfigPayload) error
	Close(ctx context.Context) error
}

// compilation type checking
var _ TaskSubmitter = (*CloudTasksSubmitter)(nil)

// CloudTasksSubmitter implements TaskSubmitter. It uses Cloud Tasks to do asynchronous calls.
// It manages submitting tasks to Google Cloud Tasks.
type CloudTasksSubmitter struct {
	client              *cloudtasks.Client
	queuePath           string
	targetURL           string
	serviceAccountEmail string
	audience            string
}

// NewCloudTasksSubmitter creates a new CloudTasksSubmitter.
// It uses `client`, `queuePath` to push requests to `targetURL`, on the behalf of `serviceAccountEmail`.
func NewCloudTasksSubmitter(client *cloudtasks.Client, queuePath, targetURL, serviceAccountEmail string, opts ...CloudTasksSubmitterOption) *CloudTasksSubmitter {
	submitter := &CloudTasksSubmitter{
		client:              client,
		queuePath:           queuePath,
		targetURL:           targetURL,
		serviceAccountEmail: serviceAccountEmail,
	}
	for _, opt := range opts {
		opt(submitter)
	}
	return submitter
}

// CloudTasksSubmitterOption is an option for CloudTasksSubmitter.
type CloudTasksSubmitterOption func(*CloudTasksSubmitter)

// WithAudience is a CloudTasksSubmitterOption that sets the audience for the task.
func WithAudience(audience string) CloudTasksSubmitterOption {
	return func(s *CloudTasksSubmitter) {
		s.audience = audience
	}
}

// CreateTask submits a task to write the configuration to storage.
func (s *CloudTasksSubmitter) CreateTask(ctx context.Context, payload *WriteConfigPayload) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	var req *taskspb.CreateTaskRequest
	audience := s.audience
	if audience == "" { // if audience is not set, use targetURL as audience
		audience = s.targetURL
	}
	if s.serviceAccountEmail == "" {
		req = &taskspb.CreateTaskRequest{
			Parent: s.queuePath,
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{
					HttpRequest: &taskspb.HttpRequest{
						HttpMethod: taskspb.HttpMethod_POST,
						Url:        s.targetURL,
						Body:       jsonPayload,
						Headers:    map[string]string{"Content-Type": "application/json"},
					},
				},
			},
		}
		slog.Default().LogAttrs(ctx, slog.LevelInfo, "created task without OIDC audience", slog.String("audience", audience))
	} else {
		req = &taskspb.CreateTaskRequest{
			Parent: s.queuePath,
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{
					HttpRequest: &taskspb.HttpRequest{
						HttpMethod: taskspb.HttpMethod_POST,
						Url:        s.targetURL,
						Body:       jsonPayload,
						Headers:    map[string]string{"Content-Type": "application/json"},
						AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
							OidcToken: &taskspb.OidcToken{
								ServiceAccountEmail: s.serviceAccountEmail,
								Audience:            audience,
							},
						},
					},
				},
			},
		}
	}

	if _, err := s.client.CreateTask(ctx, req); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}
	return nil
}

// Close closes the underlying client.
func (s *CloudTasksSubmitter) Close(ctx context.Context) error {
	// Add otel
	_ = ctx

	return s.client.Close()
}
