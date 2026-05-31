package main

import (
	"net/http"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
)

func TestMessageHooksIndexUpdateAndDeletePosts(t *testing.T) {
	engine := newTestSearchEngine(t)
	channel := testChannel()
	post := testPost(channel, "original needle", 100)
	api := &plugintest.API{}
	api.On("GetChannel", channel.Id).Return(channel, nil).Twice()
	plugin := &Plugin{engine: engine}
	plugin.API = api

	plugin.MessageHasBeenPosted(nil, post)
	assertPostHits(t, engine, model.ChannelList{channel}, "original", post.Id)

	updated := post.Clone()
	updated.Message = "updated needle"
	plugin.MessageHasBeenUpdated(nil, updated, post)
	assertPostHits(t, engine, model.ChannelList{channel}, "updated", post.Id)
	assertPostHits(t, engine, model.ChannelList{channel}, "original")

	plugin.MessageHasBeenDeleted(nil, updated)
	assertPostHits(t, engine, model.ChannelList{channel}, "updated")
	api.AssertExpectations(t)
}

func TestMessageHooksIndexAttachedFilesAndContinueAfterFileLoadFailure(t *testing.T) {
	engine := newTestSearchEngine(t)
	channel := testChannel()
	post := testPost(channel, "post with files", 100)
	missingFileID := model.NewId()
	file := &model.FileInfo{Id: model.NewId(), Name: "lease.pdf", Content: "signed packet", CreateAt: 200}
	post.FileIds = []string{missingFileID, file.Id}
	api := &plugintest.API{}
	allowWarnLogs(api)
	api.On("GetChannel", channel.Id).Return(channel, nil).Once()
	api.On("GetFileInfo", missingFileID).Return(nil, model.NewAppError("test", "missing", nil, "", http.StatusNotFound)).Once()
	api.On("GetFileInfo", file.Id).Return(file, nil).Once()
	plugin := &Plugin{engine: engine}
	plugin.API = api

	plugin.MessageHasBeenPosted(nil, post)

	if file.ChannelId != post.ChannelId || file.PostId != post.Id {
		t.Fatalf("expected file channel/post to be filled from post, got %#v", file)
	}
	ids, err := engine.SearchFiles(model.ChannelList{channel}, model.ParseSearchParams("packet", 0), 0, 10)
	if err != nil {
		t.Fatalf("search files: %v", err)
	}
	if len(ids) != 1 || ids[0] != file.Id {
		t.Fatalf("expected attached file indexed, got %#v", ids)
	}
	api.AssertExpectations(t)
}

func TestMessageDeletedRemovesAttachedFiles(t *testing.T) {
	engine := newTestSearchEngine(t)
	channel := testChannel()
	file := testFile(channel, "lease.pdf", "signed packet", 100)
	if err := engine.IndexFile(file); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	plugin := &Plugin{engine: engine}
	plugin.API = &plugintest.API{}

	plugin.MessageHasBeenDeleted(nil, &model.Post{Id: model.NewId(), FileIds: []string{file.Id}})

	ids, err := engine.SearchFiles(model.ChannelList{channel}, model.ParseSearchParams("packet", 0), 0, 10)
	if err != nil {
		t.Fatalf("search files: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected deleted file removed from index, got %#v", ids)
	}
}

func TestIndexPostAndFilesSkipsInactiveEngineAndChannelLoadFailure(t *testing.T) {
	channel := testChannel()
	inactive := &Plugin{}
	inactive.indexPostAndFiles(testPost(channel, "needle", 100))

	engine := newTestSearchEngine(t)
	api := &plugintest.API{}
	allowWarnLogs(api)
	api.On("GetChannel", channel.Id).Return(nil, model.NewAppError("test", "missing", nil, "", http.StatusNotFound)).Once()
	plugin := &Plugin{engine: engine}
	plugin.API = api

	plugin.indexPostAndFiles(testPost(channel, "needle", 100))
	assertPostHits(t, engine, model.ChannelList{channel}, "needle")
	api.AssertExpectations(t)
}

func TestOnDeactivateStopsIndexerAndEngine(t *testing.T) {
	engine := newTestSearchEngine(t)
	idx := &indexer{}
	plugin := &Plugin{engine: engine, indexer: idx}
	if err := plugin.OnDeactivate(); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if engine.Active() {
		t.Fatal("expected engine stopped")
	}
}
