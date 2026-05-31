package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func testChannel() *model.Channel {
	return &model.Channel{Id: model.NewId(), TeamId: model.NewId(), Name: "town-square", DisplayName: "Town Square"}
}

func testPost(channel *model.Channel, message string, createAt int64) *model.Post {
	return &model.Post{Id: model.NewId(), ChannelId: channel.Id, UserId: model.NewId(), CreateAt: createAt, Message: message}
}

func testFile(channel *model.Channel, name, content string, createAt int64) *model.FileInfo {
	return &model.FileInfo{Id: model.NewId(), PostId: model.NewId(), ChannelId: channel.Id, CreatorId: model.NewId(), CreateAt: createAt, Name: name, Content: content}
}

func assertPostHits(t *testing.T, engine *searchEngine, channels model.ChannelList, terms string, wantIDs ...string) {
	t.Helper()
	hits, err := engine.SearchPosts(channels, model.ParseSearchParams(terms, 0), 0, 20)
	if err != nil {
		t.Fatalf("search posts: %v", err)
	}
	if len(hits) != len(wantIDs) {
		t.Fatalf("expected post ids %#v, got %#v", wantIDs, hits)
	}
	for i, wantID := range wantIDs {
		if hits[i].ID != wantID {
			t.Fatalf("expected post ids %#v, got %#v", wantIDs, hits)
		}
	}
}
