//go:build integration

package storage

import (
	"context"
	"net"
	"os/exec"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestFirestoreStorage(t *testing.T) {
	// The user requested to assume a Linux environment with gcloud well-set.
	// We will start the firestore emulator using gcloud CLI.
	// 1. Find a free port
	var emulatorHost string
	var ln net.Listener
	var err error
	for range 3 {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			emulatorHost = ln.Addr().String()
			ln.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if emulatorHost == "" {
		t.Fatalf("failed to find a free port for emulator: %v", err)
	}

	projectID := "test-project"

	// 2. Start the emulator
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "gcloud", "emulators", "firestore", "start", "--host-port="+emulatorHost)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("failed to start gcloud emulator: %v", err)
	}

	// Ensure cleanup: stop the emulator and wait for it to exit
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	// 3. Wait for the emulator to be ready
	ready := false
	for range 20 {
		conn, err := net.DialTimeout("tcp", emulatorHost, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !ready {
		t.Fatal("timeout waiting for firestore emulator to be ready")
	}

	// 4. Setup client and run tests
	t.Setenv("FIRESTORE_EMULATOR_HOST", emulatorHost)
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create Firestore client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Use a dedicated collection for testing
	collection := "vanity_urls_test"
	s := NewFirestoreStorage(client, collection)

	// Clean up data before and after
	cleanupData := func() {
		iter := client.Collection(collection).DocumentRefs(ctx)
		for {
			ref, err := iter.Next()
			if err != nil {
				break
			}
			ref.Delete(ctx)
		}
	}
	cleanupData()
	t.Cleanup(cleanupData)

	testStorage(t, s)
}
