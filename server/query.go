package main

import (
	"strings"
	"unicode"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/mattermost/mattermost/server/public/model"
)

func buildPostQuery(channels model.ChannelList, searchParams []*model.SearchParams) query.Query {
	var termQueries []query.Query
	var notTermQueries []query.Query
	var filters []query.Query
	var notFilters []query.Query

	typeQ := bleve.NewTermQuery("")
	typeQ.SetField("Type")
	filters = append(filters, typeQ)

	for i, params := range searchParams {
		termOperator := query.MatchQueryOperatorAnd
		if searchParams[0].OrTerms {
			termOperator = query.MatchQueryOperatorOr
		}

		if i == 0 {
			addSharedFilters(params, "UserID", &filters, &notFilters)
		}

		if params.IsHashtag {
			if params.Terms != "" {
				termQueries = append(termQueries, hashtagQueries(params.Terms)...)
			}
			if params.ExcludedTerms != "" {
				notTermQueries = append(notTermQueries, hashtagQueries(params.ExcludedTerms)...)
			}
			continue
		}

		if params.Terms != "" {
			termQueries = append(termQueries, textQueries("Message", params.Terms, termOperator)...)
		}

		if params.ExcludedTerms != "" {
			notTermQueries = append(notTermQueries, textQueries("Message", params.ExcludedTerms, termOperator)...)
		}
	}

	return finishQuery(channels, searchParams[0].OrTerms, termQueries, notTermQueries, filters, notFilters)
}

func buildFileQuery(channels model.ChannelList, searchParams []*model.SearchParams) query.Query {
	var termQueries []query.Query
	var notTermQueries []query.Query
	var filters []query.Query
	var notFilters []query.Query

	for i, params := range searchParams {
		termOperator := query.MatchQueryOperatorAnd
		if searchParams[0].OrTerms {
			termOperator = query.MatchQueryOperatorOr
		}

		if i == 0 {
			addSharedFilters(params, "CreatorID", &filters, &notFilters)
			if len(params.Extensions) > 0 {
				filters = append(filters, termsDisjunction("Extension", params.Extensions))
			}
			if len(params.ExcludedExtensions) > 0 {
				notFilters = append(notFilters, termsDisjunction("Extension", params.ExcludedExtensions))
			}
		}

		if params.Terms != "" {
			termQueries = append(termQueries, fileTextQueries(params.Terms, termOperator)...)
		}

		if params.ExcludedTerms != "" {
			notTermQueries = append(notTermQueries, fileTextQueries(params.ExcludedTerms, termOperator)...)
		}
	}

	return finishQuery(channels, searchParams[0].OrTerms, termQueries, notTermQueries, filters, notFilters)
}

func hashtagQueries(input string) []query.Query {
	terms := strings.Fields(input)
	queries := make([]query.Query, 0, len(terms))
	for _, term := range terms {
		token := normalizeHashtag(term)
		if token == "" {
			continue
		}
		q := bleve.NewTermQuery(token)
		q.SetField("HashtagTokens")
		queries = append(queries, q)
	}
	return queries
}

type parsedSearchTerm struct {
	text     string
	phrase   bool
	wildcard bool
}

func parseSearchTerms(input string) []parsedSearchTerm {
	var terms []parsedSearchTerm
	var b strings.Builder
	inQuote := false
	flush := func(phrase bool) {
		text := strings.TrimSpace(b.String())
		b.Reset()
		if text == "" {
			return
		}
		terms = append(terms, parsedSearchTerm{
			text:     text,
			phrase:   phrase,
			wildcard: !phrase && strings.HasSuffix(text, "*"),
		})
	}

	for _, r := range input {
		switch {
		case r == '"':
			if inQuote {
				flush(true)
				inQuote = false
			} else {
				flush(false)
				inQuote = true
			}
		case unicode.IsSpace(r) && !inQuote:
			flush(false)
		default:
			b.WriteRune(r)
		}
	}
	flush(inQuote)

	return terms
}

func textQueries(field, input string, operator query.MatchQueryOperator) []query.Query {
	var queries []query.Query
	var terms []string
	for _, term := range parseSearchTerms(input) {
		switch {
		case term.phrase:
			q := bleve.NewMatchPhraseQuery(term.text)
			q.SetField(field)
			queries = append(queries, q)
		case term.wildcard:
			q := bleve.NewWildcardQuery(term.text)
			q.SetField(field)
			queries = append(queries, q)
		default:
			terms = append(terms, term.text)
		}
	}
	if len(terms) > 0 {
		q := bleve.NewMatchQuery(strings.Join(terms, " "))
		q.SetField(field)
		q.SetOperator(operator)
		queries = append(queries, q)
	}
	return queries
}

func fileTextQueries(input string, operator query.MatchQueryOperator) []query.Query {
	var queries []query.Query
	for _, q := range textQueries("Name", input, operator) {
		contentQ := cloneTextQueryForField(q, "Content")
		if contentQ == nil {
			queries = append(queries, q)
			continue
		}
		queries = append(queries, bleve.NewDisjunctionQuery(q, contentQ))
	}
	return queries
}

func cloneTextQueryForField(q query.Query, field string) query.Query {
	switch typed := q.(type) {
	case *query.MatchPhraseQuery:
		clone := bleve.NewMatchPhraseQuery(typed.MatchPhrase)
		clone.SetField(field)
		return clone
	case *query.WildcardQuery:
		clone := bleve.NewWildcardQuery(typed.Wildcard)
		clone.SetField(field)
		return clone
	case *query.MatchQuery:
		clone := bleve.NewMatchQuery(typed.Match)
		clone.SetField(field)
		clone.SetOperator(typed.Operator)
		return clone
	default:
		return nil
	}
}

func addSharedFilters(params *model.SearchParams, userField string, filters, notFilters *[]query.Query) {
	if len(params.InChannels) > 0 {
		*filters = append(*filters, termsDisjunction("ChannelID", params.InChannels))
	}
	if len(params.ExcludedChannels) > 0 {
		*notFilters = append(*notFilters, termsDisjunction("ChannelID", params.ExcludedChannels))
	}
	if len(params.FromUsers) > 0 {
		*filters = append(*filters, termsDisjunction(userField, params.FromUsers))
	}
	if len(params.ExcludedUsers) > 0 {
		*notFilters = append(*notFilters, termsDisjunction(userField, params.ExcludedUsers))
	}

	if params.OnDate != "" {
		before, after := params.GetOnDateMillis()
		beforeF, afterF := float64(before), float64(after)
		dateQ := bleve.NewNumericRangeQuery(&beforeF, &afterF)
		dateQ.SetField("CreateAt")
		*filters = append(*filters, dateQ)
	} else {
		if params.AfterDate != "" || params.BeforeDate != "" {
			var lowerBound, upperBound *float64
			if params.AfterDate != "" {
				v := float64(params.GetAfterDateMillis())
				lowerBound = &v
			}
			if params.BeforeDate != "" {
				v := float64(params.GetBeforeDateMillis())
				upperBound = &v
			}
			dateQ := bleve.NewNumericRangeQuery(lowerBound, upperBound)
			dateQ.SetField("CreateAt")
			*filters = append(*filters, dateQ)
		}
		if params.ExcludedAfterDate != "" {
			v := float64(params.GetExcludedAfterDateMillis())
			dateQ := bleve.NewNumericRangeQuery(&v, nil)
			dateQ.SetField("CreateAt")
			*notFilters = append(*notFilters, dateQ)
		}
		if params.ExcludedBeforeDate != "" {
			v := float64(params.GetExcludedBeforeDateMillis())
			dateQ := bleve.NewNumericRangeQuery(nil, &v)
			dateQ.SetField("CreateAt")
			*notFilters = append(*notFilters, dateQ)
		}
		if params.ExcludedDate != "" {
			before, after := params.GetExcludedDateMillis()
			beforeF, afterF := float64(before), float64(after)
			dateQ := bleve.NewNumericRangeQuery(&beforeF, &afterF)
			dateQ.SetField("CreateAt")
			*notFilters = append(*notFilters, dateQ)
		}
	}
}

func termsDisjunction(field string, terms []string) query.Query {
	queries := make([]query.Query, 0, len(terms))
	for _, term := range terms {
		termQ := bleve.NewTermQuery(term)
		termQ.SetField(field)
		queries = append(queries, termQ)
	}
	return bleve.NewDisjunctionQuery(queries...)
}

func finishQuery(channels model.ChannelList, orTerms bool, termQueries, notTermQueries, filters, notFilters []query.Query) query.Query {
	allTermsQ := bleve.NewBooleanQuery()
	allTermsQ.AddMustNot(notTermQueries...)
	if orTerms {
		allTermsQ.AddShould(termQueries...)
	} else {
		allTermsQ.AddMust(termQueries...)
	}

	root := bleve.NewBooleanQuery()
	root.AddMust(channelQuery(channels))
	if len(termQueries) > 0 || len(notTermQueries) > 0 {
		root.AddMust(allTermsQ)
	}
	if len(filters) > 0 {
		root.AddMust(bleve.NewConjunctionQuery(filters...))
	}
	if len(notFilters) > 0 {
		root.AddMustNot(notFilters...)
	}
	return root
}
