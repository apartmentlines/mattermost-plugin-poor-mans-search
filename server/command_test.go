package main

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

func TestBoldMatchedTerms(t *testing.T) {
	got := boldMatchedTerms("alpha needle beta", []string{"needle"})
	want := "alpha **needle** beta"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestChannelDisplayName(t *testing.T) {
	channel := &model.Channel{Name: "town-square", DisplayName: "Town Square"}
	if got := channelDisplayName(channel); got != "Town Square" {
		t.Fatalf("expected display name, got %q", got)
	}
}

func TestChannelDisplayNameFallsBackToName(t *testing.T) {
	channel := &model.Channel{Name: "town-square"}
	if got := channelDisplayName(channel); got != "town-square" {
		t.Fatalf("expected name fallback, got %q", got)
	}
}

func TestFormatPostResultsUsesDisplayNameAndURLName(t *testing.T) {
	team := &model.Team{Id: model.NewId(), Name: "example"}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square", DisplayName: "Town Square"}
	post := &model.Post{Id: model.NewId(), ChannelId: channel.Id, Message: "hello world"}
	list := model.NewPostList()
	list.AddPost(post)
	list.AddOrder(post.Id)

	api := &plugintest.API{}
	api.On("GetChannel", channel.Id).Return(channel, nil).Once()
	api.On("GetTeam", team.Id).Return(team, nil).Once()
	plugin := &Plugin{}
	plugin.API = api

	got := plugin.formatPostResults(list, nil, "hello")
	if !strings.Contains(got, "[~Town Square](/example/channels/town-square)") {
		t.Fatalf("expected display name and URL name in channel link, got %q", got)
	}
	if !strings.Contains(got, "[Post](/example/pl/"+post.Id+"?view=citation)") {
		t.Fatalf("expected post permalink, got %q", got)
	}
	api.AssertExpectations(t)
}

func TestFindCommandTermsMirrorsSearchSyntax(t *testing.T) {
	tests := map[string]string{
		"/find":                           "",
		"/find   ":                        "",
		"/find help":                      "help",
		"/find files packet":              "files packet",
		"/find in:town-square \"packet\"": `in:town-square "packet"`,
	}
	for command, want := range tests {
		if got := findCommandTerms(command); got != want {
			t.Fatalf("findCommandTerms(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestRegisterFindCommandIsHiddenFromAutocomplete(t *testing.T) {
	api := &plugintest.API{}
	api.On("RegisterCommand", mock.MatchedBy(func(command *model.Command) bool {
		return command.Trigger == commandTrigger &&
			command.AutoComplete == false &&
			command.AutoCompleteDesc == "" &&
			command.AutocompleteData == nil
	})).Return(nil).Once()

	plugin := &Plugin{}
	plugin.API = api
	if err := plugin.registerFindCommand(); err != nil {
		t.Fatalf("register find command: %v", err)
	}
	api.AssertExpectations(t)
}

func TestFormatCombinedResultsIncludesPostsAndFiles(t *testing.T) {
	team := &model.Team{Id: model.NewId(), Name: "example"}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square", DisplayName: "Town Square"}
	post := &model.Post{Id: model.NewId(), ChannelId: channel.Id, Message: "hello world"}
	posts := model.NewPostList()
	posts.AddPost(post)
	posts.AddOrder(post.Id)
	file := &model.FileInfo{Id: model.NewId(), ChannelId: channel.Id, Name: "packet.pdf"}
	files := model.NewFileInfoList()
	files.AddFileInfo(file)
	files.AddOrder(file.Id)

	api := &plugintest.API{}
	api.On("GetChannel", channel.Id).Return(channel, nil).Twice()
	api.On("GetTeam", team.Id).Return(team, nil).Twice()
	plugin := &Plugin{}
	plugin.API = api

	got := plugin.formatCombinedResults(posts, nil, files, "hello")
	if !strings.Contains(got, "##### Posts for `hello`") {
		t.Fatalf("expected posts section, got %q", got)
	}
	if !strings.Contains(got, "##### Files for `hello`") {
		t.Fatalf("expected files section, got %q", got)
	}
	if !strings.Contains(got, "[Post](/example/pl/"+post.Id+"?view=citation)") {
		t.Fatalf("expected post link, got %q", got)
	}
	if !strings.Contains(got, "[`packet.pdf`](/api/v4/files/"+file.Id+") in [~Town Square](/example/channels/town-square)") {
		t.Fatalf("expected linked file and channel, got %q", got)
	}
	api.AssertExpectations(t)
}

func TestFormatCombinedResultsOmitsEmptySections(t *testing.T) {
	file := &model.FileInfo{Id: model.NewId(), ChannelId: model.NewId(), Name: "packet.pdf"}
	files := model.NewFileInfoList()
	files.AddFileInfo(file)
	files.AddOrder(file.Id)

	api := &plugintest.API{}
	api.On("GetChannel", file.ChannelId).Return(&model.Channel{Name: "town-square"}, nil).Once()
	plugin := &Plugin{}
	plugin.API = api

	got := plugin.formatCombinedResults(model.NewPostList(), nil, files, "packet")
	if strings.Contains(got, "##### Posts for") {
		t.Fatalf("did not expect empty posts section, got %q", got)
	}
	if !strings.Contains(got, "##### Files for `packet`") {
		t.Fatalf("expected files section, got %q", got)
	}
	api.AssertExpectations(t)
}

func TestFormatCombinedResultsNoResults(t *testing.T) {
	plugin := &Plugin{}
	got := plugin.formatCombinedResults(model.NewPostList(), nil, model.NewFileInfoList(), "missing")
	if got != "No results for `missing`." {
		t.Fatalf("expected combined no-results message, got %q", got)
	}
}

func TestExecuteFindWithoutArgumentsGuidesToSearch(t *testing.T) {
	plugin := &Plugin{}
	resp, appErr := plugin.ExecuteCommand(nil, &model.CommandArgs{Command: "/find"})
	if appErr != nil {
		t.Fatalf("unexpected app error: %v", appErr)
	}
	if resp.ResponseType != model.CommandResponseTypeEphemeral {
		t.Fatalf("expected ephemeral response, got %q", resp.ResponseType)
	}
	if resp.Text != "Use `/search <terms>` to search messages and files." {
		t.Fatalf("expected search guidance, got %q", resp.Text)
	}
}

func TestExecuteFindSearchesPostsAndFiles(t *testing.T) {
	engine := newTestSearchEngine(t)
	userID := model.NewId()
	team := &model.Team{Id: model.NewId(), Name: "example"}
	channel := &model.Channel{Id: model.NewId(), TeamId: team.Id, Name: "town-square", DisplayName: "Town Square"}
	post := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: userID, CreateAt: 200, Message: "lease packet post"}
	file := &model.FileInfo{Id: model.NewId(), PostId: post.Id, ChannelId: channel.Id, CreatorId: userID, CreateAt: 100, Name: "lease-packet.pdf", Content: "lease packet file"}
	if err := engine.IndexPost(post, team.Id); err != nil {
		t.Fatalf("index post: %v", err)
	}
	if err := engine.IndexFile(file); err != nil {
		t.Fatalf("index file: %v", err)
	}

	api := &plugintest.API{}
	api.On("HasPermissionToTeam", userID, team.Id, model.PermissionViewTeam).Return(true).Twice()
	api.On("GetTeam", team.Id).Return(team, nil).Times(4)
	api.On("GetChannelsForTeamForUser", team.Id, userID, false).Return([]*model.Channel{channel}, nil).Twice()
	api.On("GetPost", post.Id).Return(post, nil).Once()
	api.On("GetFileInfo", file.Id).Return(file, nil).Once()
	api.On("GetChannel", channel.Id).Return(channel, nil).Twice()
	api.On("HasPermissionToChannel", userID, channel.Id, model.PermissionReadChannel).Return(true).Twice()
	plugin := &Plugin{engine: engine}
	plugin.API = api

	resp, appErr := plugin.ExecuteCommand(nil, &model.CommandArgs{
		Command: "/find lease packet",
		UserId:  userID,
		TeamId:  team.Id,
	})
	if appErr != nil {
		t.Fatalf("execute find: %v", appErr)
	}
	if resp.ResponseType != model.CommandResponseTypeEphemeral {
		t.Fatalf("expected ephemeral response, got %q", resp.ResponseType)
	}
	for _, want := range []string{
		"##### Posts for `lease packet`",
		"##### Files for `lease packet`",
		"[Post](/example/pl/" + post.Id + "?view=citation)",
		"[`lease-packet.pdf`](/api/v4/files/" + file.Id + ") in [~Town Square](/example/channels/town-square)",
	} {
		if !strings.Contains(resp.Text, want) {
			t.Fatalf("expected response to contain %q, got %q", want, resp.Text)
		}
	}
	api.AssertExpectations(t)
}

func TestPostResultExcerptTruncatesUTF8Safely(t *testing.T) {
	post := &model.Post{Id: model.NewId(), Message: strings.Repeat("世", 200)}
	got := postResultExcerpt(post, nil)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected valid UTF-8 excerpt, got %q", got)
	}
	if len([]rune(got)) != 180 {
		t.Fatalf("expected 177 runes plus ellipsis, got %d runes", len([]rune(got)))
	}
}

func TestFormatPostResultsIgnoresMissingPosts(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetChannel", mock.Anything).Maybe()
	plugin := &Plugin{}
	plugin.API = api
	got := plugin.formatPostResults(model.NewPostList(), nil, "hello")
	if got != "No message results for `hello`." {
		t.Fatalf("expected no results message, got %q", got)
	}
}
