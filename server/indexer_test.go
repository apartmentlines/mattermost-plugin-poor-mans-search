package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHistoryEntryFromStatus(t *testing.T) {
	entry := historyEntryFromStatus(indexStatus{
		ID:           "run1",
		StartedAt:    1000,
		CompletedAt:  2500,
		PostsIndexed: 12,
		FilesIndexed: 3,
		LastPostID:   "post1",
		LastFileID:   "file1",
	}, engineStatus{
		PostDocs: 20,
		FileDocs: 5,
	})

	if entry.Status != "success" {
		t.Fatalf("expected success, got %q", entry.Status)
	}
	if entry.RunTimeMillis != 1500 {
		t.Fatalf("expected runtime 1500, got %d", entry.RunTimeMillis)
	}
	if entry.PostDocs != 20 || entry.FileDocs != 5 {
		t.Fatalf("expected doc counts 20/5, got %d/%d", entry.PostDocs, entry.FileDocs)
	}
	expectedDetails := "12 posts processed, 3 files processed, index contains 20 post docs and 5 file docs"
	if entry.Details != expectedDetails {
		t.Fatalf("expected details %q, got %q", expectedDetails, entry.Details)
	}
}

func TestHistoryEntryFromStatusWithError(t *testing.T) {
	entry := historyEntryFromStatus(indexStatus{
		ID:          "run1",
		StartedAt:   1000,
		CompletedAt: 2500,
		LastError:   "failed",
	}, engineStatus{})

	if entry.Status != "error" {
		t.Fatalf("expected error, got %q", entry.Status)
	}
	if entry.Details != "failed" {
		t.Fatalf("expected error details, got %q", entry.Details)
	}
}

func TestHistoryEntryDoesNotExposeRemovedWindowFields(t *testing.T) {
	entry := historyEntryFromStatus(indexStatus{
		ID:          "run1",
		StartedAt:   1000,
		CompletedAt: 2500,
	}, engineStatus{})
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if strings.Contains(string(data), "window_starts_at") {
		t.Fatalf("expected no removed window field, got %s", string(data))
	}
}

func TestIndexerStopCancelsAndWaits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	idx := &indexer{
		cancel: cancel,
		done:   done,
	}

	go func() {
		<-ctx.Done()
		close(done)
	}()

	stopped := make(chan struct{})
	go func() {
		idx.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("expected Stop to cancel and wait for rebuild completion")
	}
}
