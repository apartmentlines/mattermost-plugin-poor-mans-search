package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

func TestPostSearchAPIResponseShape(t *testing.T) {
	engine := newTestSearchEngine(t)
	userID := model.NewId()
	team := &model.Team{Id: model.NewId(), Name: "example"}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square"}
	post := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: userID, CreateAt: 100, Message: "hello needle world"}
	if err := engine.IndexPost(post, team.Id); err != nil {
		t.Fatalf("index post: %v", err)
	}

	api := &plugintest.API{}
	allowDebugLogs(api)
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()
	api.On("GetPost", post.Id).Return(post, nil).Once()
	api.On("HasPermissionToChannel", userID, channel.Id, model.PermissionReadChannel).Return(true).Once()

	plugin := &Plugin{engine: engine}
	plugin.API = api

	body, err := json.Marshal(model.SearchParameter{
		Terms:   model.NewPointer("needle"),
		Page:    model.NewPointer(0),
		PerPage: model.NewPointer(20),
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+team.Id+"/posts/search", bytes.NewReader(body))
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()

	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var results model.PostSearchResults
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results.Order) != 1 || results.Order[0] != post.Id {
		t.Fatalf("expected post order with %s, got %#v", post.Id, results.Order)
	}
	if got := results.Matches[post.Id]; len(got) != 1 || got[0] != "needle" {
		t.Fatalf("expected term matches, got %#v", results.Matches)
	}
	if results.HasNext == nil || *results.HasNext {
		t.Fatalf("expected has_next false, got %#v", results.HasNext)
	}
	api.AssertExpectations(t)
}

func TestSearchAPIRequiresMattermostUserHeader(t *testing.T) {
	plugin := &Plugin{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts/search", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestFileSearchAPIResponseShape(t *testing.T) {
	engine := newTestSearchEngine(t)
	userID := model.NewId()
	team := &model.Team{Id: model.NewId(), Name: "example"}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square"}
	file := &model.FileInfo{Id: model.NewId(), PostId: model.NewId(), ChannelId: channel.Id, CreatorId: userID, CreateAt: 100, Name: "packet.pdf", Extension: "pdf", Content: "welcome packet"}
	if err := engine.IndexFile(file); err != nil {
		t.Fatalf("index file: %v", err)
	}

	api := &plugintest.API{}
	allowDebugLogs(api)
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()
	api.On("GetFileInfo", file.Id).Return(file, nil).Once()
	api.On("HasPermissionToChannel", userID, channel.Id, model.PermissionReadChannel).Return(true).Once()

	plugin := &Plugin{engine: engine}
	plugin.API = api

	body, err := json.Marshal(model.SearchParameter{
		Terms:   model.NewPointer("packet"),
		Page:    model.NewPointer(0),
		PerPage: model.NewPointer(20),
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+team.Id+"/files/search", bytes.NewReader(body))
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()

	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var results model.FileInfoList
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results.Order) != 1 || results.Order[0] != file.Id {
		t.Fatalf("expected file order with %s, got %#v", file.Id, results.Order)
	}
	if results.FileInfos[file.Id].Name != file.Name {
		t.Fatalf("expected file info in response, got %#v", results.FileInfos)
	}
	api.AssertExpectations(t)
}

func TestPostSearchAPIHasNextAfterSkippingStaleHit(t *testing.T) {
	engine := newTestSearchEngine(t)
	userID := model.NewId()
	team := &model.Team{Id: model.NewId(), Name: "example"}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square"}
	stale := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: userID, CreateAt: 300, Message: "needle stale"}
	visible := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: userID, CreateAt: 200, Message: "needle visible"}
	for _, post := range []*model.Post{stale, visible} {
		if err := engine.IndexPost(post, team.Id); err != nil {
			t.Fatalf("index post: %v", err)
		}
	}

	api := &plugintest.API{}
	allowDebugLogs(api)
	allowWarnLogs(api)
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()
	api.On("GetPost", stale.Id).Return(nil, model.NewAppError("test", "missing", nil, "", http.StatusNotFound)).Once()
	api.On("GetPost", visible.Id).Return(visible, nil).Once()
	api.On("HasPermissionToChannel", userID, channel.Id, model.PermissionReadChannel).Return(true).Once()

	plugin := &Plugin{engine: engine}
	plugin.API = api

	results, err := plugin.runPostSearch(userID, team.Id, model.SearchParameter{
		Terms:   model.NewPointer("needle"),
		Page:    model.NewPointer(0),
		PerPage: model.NewPointer(1),
	})
	if err != nil {
		t.Fatalf("run post search: %v", err)
	}
	if len(results.Order) != 1 || results.Order[0] != visible.Id {
		t.Fatalf("expected visible post only, got %#v", results.Order)
	}
	if results.HasNext == nil || *results.HasNext {
		t.Fatalf("expected has_next false after stale hit is skipped, got %#v", results.HasNext)
	}
	api.AssertExpectations(t)
}

func TestAdminAPIRequiresManageSystemPermission(t *testing.T) {
	userID := model.NewId()
	for name, request := range map[string]*http.Request{
		"rebuild": httptest.NewRequest(http.MethodPost, "/api/v1/index/rebuild", nil),
		"purge":   httptest.NewRequest(http.MethodPost, "/api/v1/index/purge", nil),
		"status":  httptest.NewRequest(http.MethodGet, "/api/v1/index/status", nil),
	} {
		t.Run(name, func(t *testing.T) {
			api := &plugintest.API{}
			api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(false).Once()
			plugin := &Plugin{engine: newSearchEngine(&configuration{}), indexer: &indexer{}}
			plugin.API = api
			request.Header.Set(mattermostUserIDHeader, userID)
			rec := httptest.NewRecorder()

			plugin.ServeHTTP(nil, rec, request)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", rec.Code)
			}
			api.AssertExpectations(t)
		})
	}
}

func TestRebuildIndexAPIStartsRebuild(t *testing.T) {
	userID := model.NewId()
	api := &plugintest.API{}
	allowErrorLogs(api)
	allowWarnLogs(api)
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(true).Once()
	api.On("KVGet", indexHistoryKey).Return(nil, nil).Maybe()
	api.On("KVSet", indexHistoryKey, mock.Anything).Return(nil).Maybe()
	plugin := &Plugin{
		configuration: &configuration{IndexDir: "unused", BatchSize: 1, SearchResultDisplay: searchResultDisplayInline},
		engine:        newSearchEngine(&configuration{}),
		indexer:       &indexer{},
	}
	plugin.indexer.p = plugin
	plugin.API = api

	req := httptest.NewRequest(http.MethodPost, "/api/v1/index/rebuild", nil)
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()
	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "started") {
		t.Fatalf("expected started response, got %s", rec.Body.String())
	}
	waitForIndexer(t, plugin.indexer)
	api.AssertExpectations(t)
}

func TestRebuildIndexAPIRejectsRunningRebuild(t *testing.T) {
	userID := model.NewId()
	api := &plugintest.API{}
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(true).Once()
	plugin := &Plugin{
		configuration: &configuration{IndexDir: "unused", BatchSize: 1, SearchResultDisplay: searchResultDisplayInline},
		engine:        newSearchEngine(&configuration{}),
		indexer:       &indexer{status: indexStatus{Running: true}},
	}
	plugin.API = api

	req := httptest.NewRequest(http.MethodPost, "/api/v1/index/rebuild", nil)
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()
	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	api.AssertExpectations(t)
}

func TestPurgeIndexAPIClearsIndexesHistoryAndStatus(t *testing.T) {
	engine := newTestSearchEngine(t)
	channel := testChannel()
	post := testPost(channel, "purge needle", 100)
	if err := engine.IndexPost(post, channel.TeamId); err != nil {
		t.Fatalf("seed post: %v", err)
	}
	userID := model.NewId()
	api := &plugintest.API{}
	allowInfoLogs(api)
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(true).Once()
	api.On("KVDelete", indexHistoryKey).Return(nil).Once()
	plugin := &Plugin{engine: engine, indexer: &indexer{status: indexStatus{Running: false, PostsIndexed: 1}}}
	plugin.indexer.p = plugin
	plugin.API = api

	req := httptest.NewRequest(http.MethodPost, "/api/v1/index/purge", nil)
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()
	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	assertPostHits(t, engine, model.ChannelList{channel}, "needle")
	if status := plugin.indexer.Status(); status.PostsIndexed != 0 || status.Running {
		t.Fatalf("expected reset status, got %#v", status)
	}
	api.AssertExpectations(t)
}

func TestPurgeIndexAPIRejectsRunningRebuild(t *testing.T) {
	userID := model.NewId()
	api := &plugintest.API{}
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(true).Once()
	plugin := &Plugin{engine: newSearchEngine(&configuration{}), indexer: &indexer{status: indexStatus{Running: true}}}
	plugin.API = api

	req := httptest.NewRequest(http.MethodPost, "/api/v1/index/purge", nil)
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()
	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	api.AssertExpectations(t)
}

func TestIndexStatusAndClientConfigAPIResponses(t *testing.T) {
	userID := model.NewId()
	api := &plugintest.API{}
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(true).Once()
	api.On("KVGet", indexHistoryKey).Return(mustJSON(t, []indexHistoryEntry{{ID: "run1", Status: "success"}}), nil).Once()
	plugin := &Plugin{
		configuration: &configuration{IndexDir: "data/test", BatchSize: 1, SearchResultDisplay: searchResultDisplaySidebar},
		engine:        newSearchEngine(&configuration{}),
		indexer:       &indexer{},
	}
	plugin.indexer.p = plugin
	plugin.API = api

	req := httptest.NewRequest(http.MethodGet, "/api/v1/index/status", nil)
	req.Header.Set(mattermostUserIDHeader, userID)
	rec := httptest.NewRecorder()
	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{"engine", "rebuild", "history"} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Fatalf("expected status response to include %q, got %s", key, rec.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/config/client", nil)
	req.Header.Set(mattermostUserIDHeader, userID)
	rec = httptest.NewRecorder()
	plugin.ServeHTTP(nil, rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), searchResultDisplaySidebar) {
		t.Fatalf("expected sidebar config, got %s", rec.Body.String())
	}
	api.AssertExpectations(t)
}

func waitForIndexer(t *testing.T, idx *indexer) {
	t.Helper()

	idx.mut.Lock()
	done := idx.done
	running := idx.status.Running
	idx.mut.Unlock()
	if !running {
		return
	}
	if done == nil {
		t.Fatal("indexer is running without a done channel")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for indexer")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return data
}

func allowDebugLogs(api *plugintest.API) {
	for _, count := range []int{11, 13, 19} {
		args := make([]any, count)
		for i := range args {
			args[i] = mock.Anything
		}
		api.On("LogDebug", args...).Return().Maybe()
	}
}

func allowWarnLogs(api *plugintest.API) {
	for _, count := range []int{5, 7} {
		args := make([]any, count)
		for i := range args {
			args[i] = mock.Anything
		}
		api.On("LogWarn", args...).Return().Maybe()
	}
}

func allowInfoLogs(api *plugintest.API) {
	for _, count := range []int{1, 5, 7} {
		args := make([]any, count)
		for i := range args {
			args[i] = mock.Anything
		}
		api.On("LogInfo", args...).Return().Maybe()
	}
}

func allowErrorLogs(api *plugintest.API) {
	for _, count := range []int{3} {
		args := make([]any, count)
		for i := range args {
			args[i] = mock.Anything
		}
		api.On("LogError", args...).Return().Maybe()
	}
}
