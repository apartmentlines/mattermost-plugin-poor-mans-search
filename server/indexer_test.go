package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
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

func TestIndexerIndexesPostBatchesAndTracksProgress(t *testing.T) {
	engine := newTestSearchEngine(t)
	channel := testChannel()
	active := testPost(channel, "active needle", 100)
	deleted := testPost(channel, "deleted needle", 200)
	missingTeam := testPost(channel, "missing team needle", 300)
	for _, post := range []*model.Post{deleted, missingTeam} {
		if err := engine.IndexPost(post, channel.TeamId); err != nil {
			t.Fatalf("seed post: %v", err)
		}
	}
	deleted.DeleteAt = 1

	idx := &indexer{p: &Plugin{engine: engine}}
	store := &fakeBackfillStore{
		postBatches: [][]postBackfillRow{
			{
				{Post: active, TeamID: channel.TeamId},
				{Post: deleted, TeamID: channel.TeamId},
			},
			{{Post: missingTeam}},
		},
	}

	if err := idx.indexPosts(context.Background(), store, 0, 2); err != nil {
		t.Fatalf("index posts: %v", err)
	}
	assertPostHits(t, engine, model.ChannelList{channel}, "needle", active.Id)
	status := idx.Status()
	if status.PostsIndexed != 3 || status.LastPostID != missingTeam.Id || status.LastPostTime != missingTeam.CreateAt {
		t.Fatalf("unexpected post progress: %#v", status)
	}
}

func TestIndexerIndexesFileBatchesAndTracksProgress(t *testing.T) {
	engine := newTestSearchEngine(t)
	channel := testChannel()
	first := testFile(channel, "alpha.txt", "needle", 100)
	second := testFile(channel, "beta.txt", "needle", 200)
	third := testFile(channel, "gamma.txt", "needle", 300)
	idx := &indexer{p: &Plugin{engine: engine}}
	store := &fakeBackfillStore{
		fileBatches: [][]*model.FileInfo{
			{first, second},
			{third},
		},
	}

	if err := idx.indexFiles(context.Background(), store, 0, 2); err != nil {
		t.Fatalf("index files: %v", err)
	}
	ids, err := engine.SearchFiles(model.ChannelList{channel}, model.ParseSearchParams("needle", 0), 0, 10)
	if err != nil {
		t.Fatalf("search files: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected three indexed files, got %#v", ids)
	}
	status := idx.Status()
	if status.FilesIndexed != 3 || status.LastFileID != third.Id || status.LastFileTime != third.CreateAt {
		t.Fatalf("unexpected file progress: %#v", status)
	}
}

func TestIndexerReturnsStoreErrorsAndCancellation(t *testing.T) {
	engine := newTestSearchEngine(t)
	idx := &indexer{p: &Plugin{engine: engine}}
	storeErr := errors.New("store failed")
	if err := idx.indexPosts(context.Background(), &fakeBackfillStore{postErr: storeErr}, 0, 2); !errors.Is(err, storeErr) {
		t.Fatalf("expected store error, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := idx.indexFiles(ctx, &fakeBackfillStore{}, 0, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
}

func TestIndexerStartRebuildRejectsConcurrentRunAndCancelRequiresRun(t *testing.T) {
	idx := &indexer{}
	if err := idx.Cancel(); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected not-running cancel error, got %v", err)
	}
	idx.status.Running = true
	if err := idx.StartRebuild(); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected already-running rebuild error, got %v", err)
	}
}

type fakeBackfillStore struct {
	postBatches [][]postBackfillRow
	fileBatches [][]*model.FileInfo
	postErr     error
	fileErr     error
	postCalls   int
	fileCalls   int
}

func (s *fakeBackfillStore) FetchPostBatch(_ context.Context, _ backfillCursor, _ int) ([]postBackfillRow, error) {
	if s.postErr != nil {
		return nil, s.postErr
	}
	if s.postCalls >= len(s.postBatches) {
		return nil, nil
	}
	batch := s.postBatches[s.postCalls]
	s.postCalls++
	return batch, nil
}

func (s *fakeBackfillStore) FetchFileBatch(_ context.Context, _ backfillCursor, _ int) ([]*model.FileInfo, error) {
	if s.fileErr != nil {
		return nil, s.fileErr
	}
	if s.fileCalls >= len(s.fileBatches) {
		return nil, nil
	}
	batch := s.fileBatches[s.fileCalls]
	s.fileCalls++
	return batch, nil
}
