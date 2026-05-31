package govanityurls

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/markxp/govanityurls/storage"
)

type mockTaskSubmitter struct {
	app *App
}

func (m *mockTaskSubmitter) CreateTask(ctx context.Context, payload *WriteConfigPayload) error {
	// Simulate queue execution in the background with virtual time delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = m.app.storage.Set(context.Background(), payload.Path, payload.Config)
	}()
	return nil
}

func (m *mockTaskSubmitter) Close(ctx context.Context) error {
	return nil
}

func TestDoRegisterRepo_Async(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{}}
		app := NewApp("example.com", 300, store, nil, nil, nil, nil)
		submitter := &mockTaskSubmitter{app: app}
		app.asyncCfg = &AsyncRegisterConfig{
			TaskClient: submitter,
		}

		ctx := context.Background()
		targetConfig := &storage.RepoConfig{
			Repo: "https://github.com/foo/bar",
			VCS:  "git",
		}

		err := app.doRegisterRepo(ctx, "/foo", targetConfig)
		if err != nil {
			t.Fatalf("doRegisterRepo failed: %v", err)
		}

		// Verify that it is NOT in storage yet because the background worker is sleeping (virtual time has not advanced)
		got, err := store.Get(ctx, "/foo")
		if err != nil {
			t.Fatalf("store.Get failed: %v", err)
		}
		if got != nil {
			t.Fatalf("expected config to not be in storage yet, got %v", got)
		}

		// Wait for the background goroutine to execute by sleeping (which advances virtual time)
		time.Sleep(100 * time.Millisecond)

		// Verify that the configuration is now present in the storage after time has advanced
		got, err = store.Get(ctx, "/foo")
		if err != nil {
			t.Fatalf("store.Get failed: %v", err)
		}
		if got == nil {
			t.Fatal("expected config to be present in storage after synctest.Wait()")
		}
		if got.Repo != "https://github.com/foo/bar" || got.VCS != "git" {
			t.Fatalf("unexpected stored config: got %v", got)
		}
	})
}
