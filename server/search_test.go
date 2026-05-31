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
	api.On("GetUserByUsername", "missing-channel").Return(nil, model.NewAppError("test", "not_found", nil, "", http.StatusNotFound)).Once()

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
	api.On("GetUserByUsername", "missing-channel").Return(nil, model.NewAppError("test", "not_found", nil, "", http.StatusNotFound)).Once()

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

func TestPrepareSearchResolvesChannelIDNameAndDisplayName(t *testing.T) {
	userID := model.NewId()
	team := &model.Team{Id: model.NewId()}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square", DisplayName: "Town Square"}

	for name, terms := range map[string]string{
		"id":           "needle in:" + channel.Id,
		"name":         "needle in:Town-Square",
		"display name": `needle in:"Town Square"`,
	} {
		t.Run(name, func(t *testing.T) {
			api := &plugintest.API{}
			api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
			api.On("GetTeam", team.Id).Return(team, nil).Once()
			api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Once()

			plugin := &Plugin{}
			plugin.API = api
			channels, params, err := plugin.prepareSearch(userID, team.Id, model.SearchParameter{
				Terms: model.NewPointer(terms),
			})
			if err != nil {
				t.Fatalf("prepare search: %v", err)
			}
			if len(channels) != 1 || len(params) != 1 {
				t.Fatalf("expected one search param, got channels=%#v params=%#v", channels, params)
			}
			if got := params[0].InChannels; len(got) != 1 || got[0] != channel.Id {
				t.Fatalf("expected resolved channel id %s, got %#v", channel.Id, got)
			}
			api.AssertExpectations(t)
		})
	}
}

func TestPrepareSearchResolvesDirectChannelUsername(t *testing.T) {
	userID := model.NewId()
	targetUser := &model.User{Id: model.NewId(), Username: "alice"}
	team := &model.Team{Id: model.NewId()}
	dm := &model.Channel{Id: model.NewId(), Name: model.GetDMNameFromIds(userID, targetUser.Id), Type: model.ChannelTypeDirect}
	api := &plugintest.API{}
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{dm}, nil).Once()
	api.On("GetUserByUsername", "alice").Return(targetUser, nil).Once()

	plugin := &Plugin{}
	plugin.API = api
	channels, params, err := plugin.prepareSearch(userID, team.Id, model.SearchParameter{
		Terms: model.NewPointer("needle in:alice"),
	})
	if err != nil {
		t.Fatalf("prepare search: %v", err)
	}
	if len(channels) != 1 || len(params) != 1 {
		t.Fatalf("expected one search param, got channels=%#v params=%#v", channels, params)
	}
	if got := params[0].InChannels; len(got) != 1 || got[0] != dm.Id {
		t.Fatalf("expected resolved direct channel id %s, got %#v", dm.Id, got)
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

func TestPrepareSearchUnknownNegativeUserDoesNotSuppressResults(t *testing.T) {
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
		Terms: model.NewPointer("needle -from:missing"),
	})
	if err != nil {
		t.Fatalf("prepare search: %v", err)
	}
	if len(channels) != 1 || len(params) != 1 {
		t.Fatalf("expected normal search, got channels=%#v params=%#v", channels, params)
	}
	if len(params[0].ExcludedUsers) != 0 {
		t.Fatalf("expected unknown negative user to resolve to no exclusions, got %#v", params[0].ExcludedUsers)
	}
	api.AssertExpectations(t)
}
