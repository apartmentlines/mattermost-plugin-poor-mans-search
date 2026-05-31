package main

import (
	"reflect"
	"testing"
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
