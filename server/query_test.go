package main

import (
	"reflect"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

func TestParseSearchTermsPreservesPhrasesAndWildcards(t *testing.T) {
	got := parseSearchTerms(`alpha "exact phrase" beta* "trailing phrase`)
	want := []parsedSearchTerm{
		{text: "alpha"},
		{text: "exact phrase", phrase: true},
		{text: "beta*", wildcard: true},
		{text: "trailing phrase", phrase: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestResolveChannelNames(t *testing.T) {
	channelID := model.NewId()
	channelByName := map[string][]string{
		"town-square": []string{channelID},
	}

	if got := resolveChannelNames(channelByName, []string{"Town-Square"}); !reflect.DeepEqual(got, []string{channelID}) {
		t.Fatalf("expected resolved channel id, got %#v", got)
	}
	if got := resolveChannelNames(channelByName, []string{"missing"}); len(got) != 0 {
		t.Fatalf("expected unresolved channel to return no ids, got %#v", got)
	}
}
