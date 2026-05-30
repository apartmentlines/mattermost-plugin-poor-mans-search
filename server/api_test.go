package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	if len(results.PostList.Order) != 1 || results.PostList.Order[0] != post.Id {
		t.Fatalf("expected post order with %s, got %#v", post.Id, results.PostList.Order)
	}
	if got := results.Matches[post.Id]; len(got) != 1 || got[0] != "needle" {
		t.Fatalf("expected term matches, got %#v", results.Matches)
	}
	if results.PostList.HasNext == nil || *results.PostList.HasNext {
		t.Fatalf("expected has_next false, got %#v", results.PostList.HasNext)
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
	if len(results.PostList.Order) != 1 || results.PostList.Order[0] != visible.Id {
		t.Fatalf("expected visible post only, got %#v", results.PostList.Order)
	}
	if results.PostList.HasNext == nil || *results.PostList.HasNext {
		t.Fatalf("expected has_next false after stale hit is skipped, got %#v", results.PostList.HasNext)
	}
	api.AssertExpectations(t)
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
	for _, count := range []int{5} {
		args := make([]any, count)
		for i := range args {
			args[i] = mock.Anything
		}
		api.On("LogWarn", args...).Return().Maybe()
	}
}
