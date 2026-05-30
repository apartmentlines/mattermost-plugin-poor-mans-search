package main

import (
	"net/http"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
)

func TestPrepareSearchUnknownPositiveChannelReturnsNoResults(t *testing.T) {
	userID := model.NewId()
	team := &model.Team{Id: model.NewId()}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square"}
	api := &plugintest.API{}
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()

	plugin := &Plugin{}
	plugin.API = api
	channels, params, err := plugin.prepareSearch(userID, team.Id, model.SearchParameter{
		Terms: model.NewPointer("needle in:missing-channel"),
	})
	if err != nil {
		t.Fatalf("prepare search: %v", err)
	}
	if len(channels) != 0 || len(params) != 0 {
		t.Fatalf("expected no-results search, got channels=%#v params=%#v", channels, params)
	}
	api.AssertExpectations(t)
}

func TestPrepareSearchUnknownNegativeChannelDoesNotSuppressResults(t *testing.T) {
	userID := model.NewId()
	team := &model.Team{Id: model.NewId()}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square"}
	api := &plugintest.API{}
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()

	plugin := &Plugin{}
	plugin.API = api
	channels, params, err := plugin.prepareSearch(userID, team.Id, model.SearchParameter{
		Terms: model.NewPointer("needle -in:missing-channel"),
	})
	if err != nil {
		t.Fatalf("prepare search: %v", err)
	}
	if len(channels) != 1 || len(params) != 1 {
		t.Fatalf("expected normal search, got channels=%#v params=%#v", channels, params)
	}
	if len(params[0].ExcludedChannels) != 0 {
		t.Fatalf("expected unknown negative channel to resolve to no exclusions, got %#v", params[0].ExcludedChannels)
	}
	api.AssertExpectations(t)
}

func TestPrepareSearchUnknownPositiveUserReturnsNoResults(t *testing.T) {
	userID := model.NewId()
	team := &model.Team{Id: model.NewId()}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square"}
	api := &plugintest.API{}
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()
	api.On("GetUserByUsername", "missing").Return(nil, model.NewAppError("test", "not_found", nil, "", http.StatusNotFound)).Once()

	plugin := &Plugin{}
	plugin.API = api
	channels, params, err := plugin.prepareSearch(userID, team.Id, model.SearchParameter{
		Terms: model.NewPointer("needle from:missing"),
	})
	if err != nil {
		t.Fatalf("prepare search: %v", err)
	}
	if len(channels) != 0 || len(params) != 0 {
		t.Fatalf("expected no-results search, got channels=%#v params=%#v", channels, params)
	}
	api.AssertExpectations(t)
}
