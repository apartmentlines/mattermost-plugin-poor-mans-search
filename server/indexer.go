package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
)

const indexHistoryKey = "index_history"

type indexStatus struct {
	ID           string `json:"id,omitempty"`
	Running      bool   `json:"running"`
	LastError    string `json:"last_error,omitempty"`
	PostsIndexed int64  `json:"posts_indexed"`
	FilesIndexed int64  `json:"files_indexed"`
	StartedAt    int64  `json:"started_at,omitempty"`
	CompletedAt  int64  `json:"completed_at,omitempty"`
	LastPostTime int64  `json:"last_post_time,omitempty"`
	LastPostID   string `json:"last_post_id,omitempty"`
	LastFileTime int64  `json:"last_file_time,omitempty"`
	LastFileID   string `json:"last_file_id,omitempty"`
}

type indexHistoryEntry struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	StartedAt     int64  `json:"started_at"`
	CompletedAt   int64  `json:"completed_at,omitempty"`
	RunTimeMillis int64  `json:"run_time_millis,omitempty"`
	PostsIndexed  int64  `json:"posts_indexed"`
	FilesIndexed  int64  `json:"files_indexed"`
	PostDocs      uint64 `json:"post_docs"`
	FileDocs      uint64 `json:"file_docs"`
	LastPostTime  int64  `json:"last_post_time,omitempty"`
	LastPostID    string `json:"last_post_id,omitempty"`
	LastFileTime  int64  `json:"last_file_time,omitempty"`
	LastFileID    string `json:"last_file_id,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	Details       string `json:"details,omitempty"`
}

type indexer struct {
	p      *Plugin
	mut    sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	status indexStatus
}

func newIndexer(p *Plugin) *indexer {
	return &indexer{p: p}
}

func (i *indexer) Status() indexStatus {
	i.mut.Lock()
	defer i.mut.Unlock()
	return i.status
}

func (i *indexer) ResetStatus() {
	i.mut.Lock()
	defer i.mut.Unlock()
	i.status = indexStatus{}
}

func (i *indexer) StartRebuild() error {
	i.mut.Lock()
	if i.status.Running {
		i.mut.Unlock()
		return errors.New("index rebuild is already running")
	}
	cfg := i.p.getConfiguration()
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel
	i.done = make(chan struct{})
	i.status = indexStatus{ID: model.NewId(), Running: true, StartedAt: model.GetMillis()}
	i.mut.Unlock()

	go i.rebuild(ctx, cfg)
	return nil
}

func (i *indexer) rebuild(ctx context.Context, cfg *configuration) {
	defer func() {
		i.mut.Lock()
		done := i.done
		i.done = nil
		i.mut.Unlock()
		if done != nil {
			close(done)
		}
	}()

	err := i.runRebuild(ctx, cfg)
	currentEngineStatus := i.p.engine.Status()
	i.mut.Lock()
	i.status.Running = false
	i.status.CompletedAt = model.GetMillis()
	i.cancel = nil
	if err != nil {
		i.status.LastError = err.Error()
		i.p.API.LogError("Search index rebuild failed", "error", err.Error())
	} else {
		i.status.LastError = ""
		i.p.API.LogInfo("Search index rebuild completed", "posts_indexed", i.status.PostsIndexed, "files_indexed", i.status.FilesIndexed)
	}
	entry := historyEntryFromStatus(i.status, currentEngineStatus)
	i.mut.Unlock()

	if saveErr := i.prependHistory(entry); saveErr != nil {
		i.p.API.LogWarn("Failed to save search index rebuild history", "error", saveErr.Error())
	}
}

func (i *indexer) runRebuild(ctx context.Context, cfg *configuration) error {
	if i.p.engine == nil || !i.p.engine.Active() {
		return errors.New("search index is not active")
	}
	db, err := i.p.client.Store.GetReplicaDB()
	if err != nil {
		return err
	}
	store, err := newBackfillStore(db, i.p.client.Store.DriverName(), i.p.API.GetServerVersion())
	if err != nil {
		return err
	}
	startAt := int64(0)
	if err := i.indexPosts(ctx, store, startAt, cfg.BatchSize); err != nil {
		return err
	}
	return i.indexFiles(ctx, store, startAt, cfg.BatchSize)
}

func (i *indexer) indexPosts(ctx context.Context, store backfillStore, startAt int64, batchSize int) error {
	cursor := backfillCursor{CreateAt: startAt}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		posts, err := store.FetchPostBatch(ctx, cursor, batchSize)
		if err != nil {
			return err
		}

		for _, row := range posts {
			if row.Post.DeleteAt == 0 && row.TeamID != "" {
				if err := i.p.engine.IndexPost(row.Post, row.TeamID); err != nil {
					return err
				}
			} else if err := i.p.engine.DeletePost(row.Post.Id); err != nil {
				return err
			}
			cursor.CreateAt = row.Post.CreateAt
			cursor.ID = row.Post.Id
		}
		i.addPostProgress(int64(len(posts)), cursor.CreateAt, cursor.ID)
		if len(posts) < batchSize {
			return nil
		}
	}
}

func (i *indexer) indexFiles(ctx context.Context, store backfillStore, startAt int64, batchSize int) error {
	cursor := backfillCursor{CreateAt: startAt}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		files, err := store.FetchFileBatch(ctx, cursor, batchSize)
		if err != nil {
			return err
		}

		for _, file := range files {
			if err := i.p.engine.IndexFile(file); err != nil {
				return err
			}
			cursor.CreateAt = file.CreateAt
			cursor.ID = file.Id
		}
		i.addFileProgress(int64(len(files)), cursor.CreateAt, cursor.ID)
		if len(files) < batchSize {
			return nil
		}
	}
}

func (i *indexer) addPostProgress(count, lastTime int64, lastID string) {
	i.mut.Lock()
	defer i.mut.Unlock()
	i.status.PostsIndexed += count
	i.status.LastPostTime = lastTime
	i.status.LastPostID = lastID
}

func (i *indexer) addFileProgress(count, lastTime int64, lastID string) {
	i.mut.Lock()
	defer i.mut.Unlock()
	i.status.FilesIndexed += count
	i.status.LastFileTime = lastTime
	i.status.LastFileID = lastID
}

func (i *indexer) Cancel() error {
	i.mut.Lock()
	defer i.mut.Unlock()
	if i.cancel == nil {
		return fmt.Errorf("index rebuild is not running")
	}
	i.cancel()
	return nil
}

func (i *indexer) Stop() {
	i.mut.Lock()
	cancel := i.cancel
	done := i.done
	i.mut.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (i *indexer) ClearHistory() error {
	if appErr := i.p.API.KVDelete(indexHistoryKey); appErr != nil {
		return appErr
	}
	return nil
}

func (i *indexer) History() ([]indexHistoryEntry, error) {
	var history []indexHistoryEntry
	data, appErr := i.p.API.KVGet(indexHistoryKey)
	if appErr != nil {
		return nil, appErr
	}
	if len(data) == 0 {
		return history, nil
	}
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}

func (i *indexer) prependHistory(entry indexHistoryEntry) error {
	history, err := i.History()
	if err != nil {
		return err
	}
	history = append([]indexHistoryEntry{entry}, history...)
	if len(history) > 10 {
		history = history[:10]
	}
	data, err := json.Marshal(history)
	if err != nil {
		return err
	}
	if appErr := i.p.API.KVSet(indexHistoryKey, data); appErr != nil {
		return appErr
	}
	return nil
}

func historyEntryFromStatus(status indexStatus, engineStatus engineStatus) indexHistoryEntry {
	entry := indexHistoryEntry{
		ID:           status.ID,
		Status:       "success",
		StartedAt:    status.StartedAt,
		CompletedAt:  status.CompletedAt,
		PostsIndexed: status.PostsIndexed,
		FilesIndexed: status.FilesIndexed,
		PostDocs:     engineStatus.PostDocs,
		FileDocs:     engineStatus.FileDocs,
		LastPostTime: status.LastPostTime,
		LastPostID:   status.LastPostID,
		LastFileTime: status.LastFileTime,
		LastFileID:   status.LastFileID,
		LastError:    status.LastError,
	}
	if entry.CompletedAt > 0 && entry.StartedAt > 0 {
		entry.RunTimeMillis = entry.CompletedAt - entry.StartedAt
	}
	if status.LastError != "" {
		entry.Status = "error"
		entry.Details = status.LastError
	} else {
		entry.Details = fmt.Sprintf("%d posts processed, %d files processed, index contains %d post docs and %d file docs", entry.PostsIndexed, entry.FilesIndexed, entry.PostDocs, entry.FileDocs)
	}
	return entry
}
