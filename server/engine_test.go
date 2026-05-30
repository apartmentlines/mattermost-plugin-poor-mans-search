package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestSearchEngineIndexesAndSearchesPosts(t *testing.T) {
	engine := newTestSearchEngine(t)

	channel := &model.Channel{Id: model.NewId(), TeamId: model.NewId()}
	matching := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: model.NewId(), CreateAt: 200, Message: "needle in haystack"}
	other := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: model.NewId(), CreateAt: 100, Message: "something else"}

	if err := engine.IndexPost(matching, channel.TeamId); err != nil {
		t.Fatalf("index matching post: %v", err)
	}
	if err := engine.IndexPost(other, channel.TeamId); err != nil {
		t.Fatalf("index other post: %v", err)
	}

	params := model.ParseSearchParams("needle", 0)
	hits, err := engine.SearchPosts(model.ChannelList{channel}, params, 0, 10)
	if err != nil {
		t.Fatalf("search posts: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != matching.Id {
		t.Fatalf("expected only matching post %s, got %#v", matching.Id, hits)
	}
	if len(hits[0].Matches) != 1 || hits[0].Matches[0] != "needle" {
		t.Fatalf("expected highlighted search match, got %#v", hits[0].Matches)
	}

	status := engine.Status()
	if !status.Active {
		t.Fatal("expected engine to be active")
	}
	if status.PostDocs != 2 {
		t.Fatalf("expected 2 indexed post docs, got %d", status.PostDocs)
	}
}

func TestSearchEnginePhraseSearchesPosts(t *testing.T) {
	engine := newTestSearchEngine(t)

	channel := &model.Channel{Id: model.NewId(), TeamId: model.NewId()}
	matching := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: model.NewId(), CreateAt: 300, Message: "alpha exact phrase beta"}
	reversed := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: model.NewId(), CreateAt: 200, Message: "alpha phrase exact beta"}
	separate := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: model.NewId(), CreateAt: 100, Message: "exact words but not the phrase"}

	for _, post := range []*model.Post{matching, reversed, separate} {
		if err := engine.IndexPost(post, channel.TeamId); err != nil {
			t.Fatalf("index post: %v", err)
		}
	}

	params := model.ParseSearchParams(`"exact phrase"`, 0)
	hits, err := engine.SearchPosts(model.ChannelList{channel}, params, 0, 10)
	if err != nil {
		t.Fatalf("search posts: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != matching.Id {
		t.Fatalf("expected only exact phrase post %s, got %#v", matching.Id, hits)
	}
}

func TestSearchEnginePostFilters(t *testing.T) {
	engine := newTestSearchEngine(t)

	userID := model.NewId()
	otherUserID := model.NewId()
	channel := &model.Channel{Id: model.NewId(), TeamId: model.NewId()}
	otherChannel := &model.Channel{Id: model.NewId(), TeamId: channel.TeamId}
	matching := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: userID, CreateAt: 200, Message: "release needle", Hashtags: "#ship"}
	wrongUser := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: otherUserID, CreateAt: 210, Message: "release needle", Hashtags: "#ship"}
	wrongChannel := &model.Post{Id: model.NewId(), ChannelId: otherChannel.Id, UserId: userID, CreateAt: 220, Message: "release needle", Hashtags: "#ship"}
	excluded := &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: userID, CreateAt: 230, Message: "release needle archived", Hashtags: "#ship"}

	for _, post := range []*model.Post{matching, wrongUser, wrongChannel, excluded} {
		if err := engine.IndexPost(post, channel.TeamId); err != nil {
			t.Fatalf("index post: %v", err)
		}
	}

	params := []*model.SearchParams{{
		Terms:            "release needle",
		ExcludedTerms:    "archived",
		InChannels:       []string{channel.Id},
		FromUsers:        []string{userID},
		ExcludedChannels: []string{model.NewId()},
	}}
	hits, err := engine.SearchPosts(model.ChannelList{channel, otherChannel}, params, 0, 10)
	if err != nil {
		t.Fatalf("search posts: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != matching.Id {
		t.Fatalf("expected only filtered post %s, got %#v", matching.Id, hits)
	}

	hashtagParams := []*model.SearchParams{{Terms: "#ship", IsHashtag: true, InChannels: []string{channel.Id}}}
	hits, err = engine.SearchPosts(model.ChannelList{channel, otherChannel}, hashtagParams, 0, 10)
	if err != nil {
		t.Fatalf("search hashtag posts: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("expected three #ship posts in channel, got %#v", hits)
	}
}

func TestHighlightedTermsExtractsMarkedTerms(t *testing.T) {
	fragments := []string{
		"<mark>Apples</mark> and oranges and <mark>apple</mark>",
		"Johnny <mark>Appleseed</mark>",
		"That does not <mark>apply</mark> to me",
	}
	expected := []string{"Apples", "apple", "Appleseed", "apply"}

	actual := highlightedTerms(fragments, "mark")
	if len(actual) != len(expected) {
		t.Fatalf("expected %#v, got %#v", expected, actual)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("expected %#v, got %#v", expected, actual)
		}
	}
}

func TestSearchEngineIndexesAndSearchesFiles(t *testing.T) {
	engine := newTestSearchEngine(t)

	channel := &model.Channel{Id: model.NewId(), TeamId: model.NewId()}
	matching := &model.FileInfo{Id: model.NewId(), PostId: model.NewId(), ChannelId: channel.Id, CreatorId: model.NewId(), CreateAt: 200, Name: "budget.xlsx", Extension: "xlsx", Content: "quarterly revenue"}
	other := &model.FileInfo{Id: model.NewId(), PostId: model.NewId(), ChannelId: channel.Id, CreatorId: model.NewId(), CreateAt: 100, Name: "notes.txt", Extension: "txt", Content: "nothing relevant"}

	if err := engine.IndexFile(matching); err != nil {
		t.Fatalf("index matching file: %v", err)
	}
	if err := engine.IndexFile(other); err != nil {
		t.Fatalf("index other file: %v", err)
	}

	params := model.ParseSearchParams("budget", 0)
	ids, err := engine.SearchFiles(model.ChannelList{channel}, params, 0, 10)
	if err != nil {
		t.Fatalf("search files: %v", err)
	}
	if len(ids) != 1 || ids[0] != matching.Id {
		t.Fatalf("expected only matching file %s, got %#v", matching.Id, ids)
	}
}

func TestSearchEnginePhraseSearchesFiles(t *testing.T) {
	engine := newTestSearchEngine(t)

	channel := &model.Channel{Id: model.NewId(), TeamId: model.NewId()}
	matching := &model.FileInfo{Id: model.NewId(), PostId: model.NewId(), ChannelId: channel.Id, CreatorId: model.NewId(), CreateAt: 200, Name: "welcome-packet.pdf", Extension: "pdf", Content: "tenant welcome packet"}
	reversed := &model.FileInfo{Id: model.NewId(), PostId: model.NewId(), ChannelId: channel.Id, CreatorId: model.NewId(), CreateAt: 100, Name: "packet-welcome.pdf", Extension: "pdf", Content: "packet welcome tenant"}

	for _, file := range []*model.FileInfo{matching, reversed} {
		if err := engine.IndexFile(file); err != nil {
			t.Fatalf("index file: %v", err)
		}
	}

	params := model.ParseSearchParams(`"welcome packet"`, 0)
	ids, err := engine.SearchFiles(model.ChannelList{channel}, params, 0, 10)
	if err != nil {
		t.Fatalf("search files: %v", err)
	}
	if len(ids) != 1 || ids[0] != matching.Id {
		t.Fatalf("expected only exact phrase file %s, got %#v", matching.Id, ids)
	}
}

func newTestSearchEngine(t *testing.T) *searchEngine {
	t.Helper()
	engine := newSearchEngine(&configuration{
		IndexDir:  t.TempDir(),
		BatchSize: 1,
	})
	if err := engine.Start(); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Stop(); err != nil {
			t.Fatalf("stop engine: %v", err)
		}
	})
	return engine
}
