package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

func (p *Plugin) runPostSearch(userID, teamID string, params model.SearchParameter) (*model.PostSearchResults, error) {
	page, perPage := searchPage(params, 60)
	channels, finalParams, err := p.prepareSearch(userID, teamID, params)
	if err != nil {
		return nil, err
	}
	list := model.NewPostList()
	matches := model.PostSearchMatches{}
	hasNext := false
	if len(channels) > 0 && len(finalParams) > 0 {
		visible, err := p.visiblePostHits(userID, channels, finalParams, page, perPage)
		if err != nil {
			return nil, err
		}
		hasNext = len(visible) > perPage
		if hasNext {
			visible = visible[:perPage]
		}
		for _, result := range visible {
			list.AddPost(result.post)
			list.AddOrder(result.post.Id)
			if len(result.matches) > 0 {
				matches[result.post.Id] = result.matches
			}
		}
	}
	list.HasNext = model.NewPointer(hasNext)
	return model.MakePostSearchResults(list, matches), nil
}

type visiblePostHit struct {
	post    *model.Post
	matches []string
}

func (p *Plugin) visiblePostHits(userID string, channels model.ChannelList, finalParams []*model.SearchParams, page, perPage int) ([]visiblePostHit, error) {
	skip := page * perPage
	target := perPage + 1
	offset := 0
	batchSize := target
	if batchSize < 20 {
		batchSize = 20
	}
	var visible []visiblePostHit
	for len(visible) < target {
		hits, err := p.engine.SearchPosts(channels, finalParams, offset, batchSize)
		if err != nil {
			return nil, err
		}
		if len(hits) == 0 {
			break
		}
		offset += len(hits)
		for _, hit := range hits {
			post, appErr := p.API.GetPost(hit.ID)
			if appErr != nil {
				p.API.LogWarn("Failed to load search result post", "post_id", hit.ID, "error", appErr.Error())
				continue
			}
			if !p.API.HasPermissionToChannel(userID, post.ChannelId, model.PermissionReadChannel) {
				continue
			}
			if skip > 0 {
				skip--
				continue
			}
			visible = append(visible, visiblePostHit{post: post, matches: hit.Matches})
			if len(visible) >= target {
				break
			}
		}
		if len(hits) < batchSize {
			break
		}
	}
	return visible, nil
}

func (p *Plugin) runFileSearch(userID, teamID string, params model.SearchParameter) (*model.FileInfoList, error) {
	page, perPage := searchPage(params, 60)
	channels, finalParams, err := p.prepareSearch(userID, teamID, params)
	if err != nil {
		return nil, err
	}
	list := model.NewFileInfoList()
	if len(channels) > 0 && len(finalParams) > 0 {
		files, err := p.visibleFiles(userID, channels, finalParams, page, perPage)
		if err != nil {
			return nil, err
		}
		if len(files) > perPage {
			files = files[:perPage]
		}
		for _, file := range files {
			list.AddFileInfo(file)
			list.AddOrder(file.Id)
		}
	}
	return list, nil
}

func (p *Plugin) visibleFiles(userID string, channels model.ChannelList, finalParams []*model.SearchParams, page, perPage int) ([]*model.FileInfo, error) {
	skip := page * perPage
	target := perPage + 1
	offset := 0
	batchSize := target
	if batchSize < 20 {
		batchSize = 20
	}
	var visible []*model.FileInfo
	for len(visible) < target {
		ids, err := p.engine.SearchFiles(channels, finalParams, offset, batchSize)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		offset += len(ids)
		for _, id := range ids {
			file, appErr := p.API.GetFileInfo(id)
			if appErr != nil {
				p.API.LogWarn("Failed to load search result file", "file_id", id, "error", appErr.Error())
				continue
			}
			if !p.API.HasPermissionToChannel(userID, file.ChannelId, model.PermissionReadChannel) {
				continue
			}
			if skip > 0 {
				skip--
				continue
			}
			visible = append(visible, file)
			if len(visible) >= target {
				break
			}
		}
		if len(ids) < batchSize {
			break
		}
	}
	return visible, nil
}

func (p *Plugin) prepareSearch(userID, teamID string, params model.SearchParameter) (model.ChannelList, []*model.SearchParams, error) {
	terms := ""
	if params.Terms != nil {
		terms = strings.TrimSpace(*params.Terms)
	}
	if terms == "" {
		return nil, nil, fmt.Errorf("terms are required")
	}
	if terms == "*" {
		return model.ChannelList{}, []*model.SearchParams{}, nil
	}

	timeZoneOffset := 0
	if params.TimeZoneOffset != nil {
		timeZoneOffset = *params.TimeZoneOffset
	}
	isOrSearch := false
	if params.IsOrSearch != nil {
		isOrSearch = *params.IsOrSearch
	}
	includeDeletedChannels := false
	if params.IncludeDeletedChannels != nil {
		includeDeletedChannels = *params.IncludeDeletedChannels
	}

	channels, err := p.accessibleChannels(userID, teamID, includeDeletedChannels)
	if err != nil {
		return nil, nil, err
	}
	channelResolver := newChannelResolver(channels)

	paramsList := model.ParseSearchParams(terms, timeZoneOffset)
	finalParams := make([]*model.SearchParams, 0, len(paramsList))
	for _, sp := range paramsList {
		hadInChannels := len(sp.InChannels) > 0
		hadFromUsers := len(sp.FromUsers) > 0
		sp.OrTerms = isOrSearch
		sp.IncludeDeletedChannels = includeDeletedChannels
		if sp.Terms == "*" {
			continue
		}
		sp.InChannels = p.resolveChannels(userID, channelResolver, sp.InChannels)
		if hadInChannels && len(sp.InChannels) == 0 {
			return model.ChannelList{}, []*model.SearchParams{}, nil
		}
		sp.ExcludedChannels = p.resolveChannels(userID, channelResolver, sp.ExcludedChannels)
		sp.FromUsers = p.resolveUserNames(sp.FromUsers)
		if hadFromUsers && len(sp.FromUsers) == 0 {
			return model.ChannelList{}, []*model.SearchParams{}, nil
		}
		sp.ExcludedUsers = p.resolveUserNames(sp.ExcludedUsers)
		finalParams = append(finalParams, sp)
	}
	return channels, finalParams, nil
}

func searchPage(params model.SearchParameter, defaultPerPage int) (int, int) {
	page := 0
	if params.Page != nil && *params.Page > 0 {
		page = *params.Page
	}
	perPage := defaultPerPage
	if params.PerPage != nil && *params.PerPage > 0 {
		perPage = *params.PerPage
	}
	if perPage > 200 {
		perPage = 200
	}
	return page, perPage
}

func (p *Plugin) accessibleChannels(userID, teamID string, includeDeleted bool) (model.ChannelList, error) {
	var teams []*model.Team
	if teamID != "" {
		if !p.API.HasPermissionToTeam(userID, teamID, model.PermissionViewTeam) {
			return nil, model.NewAppError("accessibleChannels", "api.context.permissions.app_error", nil, "", http.StatusForbidden)
		}
		team, appErr := p.API.GetTeam(teamID)
		if appErr != nil {
			return nil, appErr
		}
		teams = []*model.Team{team}
	} else {
		var appErr *model.AppError
		teams, appErr = p.API.GetTeamsForUser(userID)
		if appErr != nil {
			return nil, appErr
		}
	}

	channelsByID := map[string]*model.Channel{}
	for _, team := range teams {
		channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, userID, includeDeleted)
		if appErr != nil {
			return nil, appErr
		}
		for _, channel := range channels {
			if channel.DeleteAt == 0 || includeDeleted {
				channelsByID[channel.Id] = channel
			}
		}
	}
	ids := make([]string, 0, len(channelsByID))
	for id := range channelsByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	channels := make(model.ChannelList, 0, len(ids))
	for _, id := range ids {
		channels = append(channels, channelsByID[id])
	}
	return channels, nil
}

type channelResolver struct {
	ids      map[string]bool
	byLookup map[string][]string
}

func newChannelResolver(channels model.ChannelList) *channelResolver {
	resolver := &channelResolver{
		ids:      map[string]bool{},
		byLookup: map[string][]string{},
	}
	for _, channel := range channels {
		resolver.ids[channel.Id] = true
		resolver.add(channel.Id, channel.Id)
		resolver.add(channel.Name, channel.Id)
		resolver.add(channel.DisplayName, channel.Id)
	}
	return resolver
}

func (r *channelResolver) add(value, channelID string) {
	value = normalizeSearchModifierValue(value)
	if value == "" {
		return
	}
	r.byLookup[value] = appendUniqueString(r.byLookup[value], channelID)
}

func (p *Plugin) resolveChannels(currentUserID string, resolver *channelResolver, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var ids []string
	for _, value := range values {
		resolved := resolver.resolve(value)
		if len(resolved) == 0 {
			resolved = p.resolveDirectChannel(currentUserID, resolver, value)
		}
		ids = appendUniqueStrings(ids, resolved...)
	}
	return ids
}

func (r *channelResolver) resolve(value string) []string {
	value = normalizeSearchModifierValue(value)
	if value == "" {
		return nil
	}
	return r.byLookup[value]
}

func (p *Plugin) resolveDirectChannel(currentUserID string, resolver *channelResolver, value string) []string {
	value = strings.TrimPrefix(normalizeSearchModifierValue(value), "@")
	if value == "" {
		return nil
	}
	user, appErr := p.API.GetUserByUsername(value)
	if appErr != nil {
		return nil
	}
	dmName := normalizeSearchModifierValue(model.GetDMNameFromIds(currentUserID, user.Id))
	for _, channelID := range resolver.byLookup[dmName] {
		if resolver.ids[channelID] {
			return []string{channelID}
		}
	}
	return nil
}

func normalizeSearchModifierValue(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		values = appendUniqueString(values, addition)
	}
	return values
}

func appendUniqueString(values []string, addition string) []string {
	if addition == "" {
		return values
	}
	for _, value := range values {
		if value == addition {
			return values
		}
	}
	return append(values, addition)
}

func (p *Plugin) resolveUserNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		user, appErr := p.API.GetUserByUsername(name)
		if appErr != nil {
			continue
		}
		ids = append(ids, user.Id)
	}
	return ids
}
