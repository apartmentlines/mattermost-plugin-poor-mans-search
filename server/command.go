package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const commandTrigger = "find"

func (p *Plugin) registerFindCommand() error {
	return p.API.RegisterCommand(&model.Command{
		Trigger:      commandTrigger,
		DisplayName:  "Find",
		Description:  "Search messages and files with the Poor Man's Search Bleve index.",
		AutoComplete: false,
	})
}

func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	text := findCommandTerms(args.Command)
	if text == "" {
		return &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: "Use `/search <terms>` to search messages and files."}, nil
	}

	params := model.SearchParameter{
		Terms:   model.NewPointer(text),
		Page:    model.NewPointer(0),
		PerPage: model.NewPointer(10),
	}
	postResults, err := p.runPostSearch(args.UserId, args.TeamId, params)
	if err != nil {
		return commandError(err), nil
	}
	fileResults, err := p.runFileSearch(args.UserId, args.TeamId, params)
	if err != nil {
		return commandError(err), nil
	}
	return &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: p.formatCombinedResults(postResults.PostList, postResults.Matches, fileResults, text)}, nil
}

func findCommandTerms(command string) string {
	return strings.TrimSpace(strings.TrimPrefix(command, "/"+commandTrigger))
}

func commandError(err error) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("Search failed: %v", err),
	}
}

func (p *Plugin) formatPostResults(list *model.PostList, matches model.PostSearchMatches, terms string) string {
	section, ok := p.postResultsSection(list, matches, terms)
	if !ok {
		return fmt.Sprintf("No message results for `%s`.", terms)
	}
	return section
}

func (p *Plugin) formatCombinedResults(posts *model.PostList, matches model.PostSearchMatches, files *model.FileInfoList, terms string) string {
	var sections []string
	if section, ok := p.postResultsSection(posts, matches, terms); ok {
		sections = append(sections, section)
	}
	if section, ok := p.fileResultsSection(files, terms); ok {
		sections = append(sections, section)
	}
	if len(sections) == 0 {
		return fmt.Sprintf("No results for `%s`.", terms)
	}
	return strings.Join(sections, "\n")
}

func (p *Plugin) postResultsSection(list *model.PostList, matches model.PostSearchMatches, terms string) (string, bool) {
	if list == nil || len(list.Order) == 0 {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "##### Posts for `%s`\n", terms)
	count := 0
	for _, id := range list.Order {
		post := list.Posts[id]
		if post == nil {
			continue
		}
		count++
		channelText, teamName := p.channelResultText(post.ChannelId)
		excerpt := postResultExcerpt(post, matches)
		if teamName != "" {
			fmt.Fprintf(&b, "- [Post](%s) in %s: %s\n", fmt.Sprintf("/%s/pl/%s?view=citation", teamName, post.Id), channelText, excerpt)
		} else {
			fmt.Fprintf(&b, "- Post in %s: %s\n", channelText, excerpt)
		}
	}
	if count == 0 {
		return "", false
	}
	return b.String(), true
}

func postResultExcerpt(post *model.Post, matches model.PostSearchMatches) string {
	excerpt := strings.TrimSpace(post.Message)
	if runeLen(excerpt) > 180 {
		excerpt = truncateRunes(excerpt, 177) + "..."
	}
	if matches == nil || len(matches[post.Id]) == 0 {
		return excerpt
	}
	return boldMatchedTerms(excerpt, matches[post.Id])
}

func truncateRunes(text string, limit int) string {
	if limit < 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func runeLen(text string) int {
	return len([]rune(text))
}

func boldMatchedTerms(text string, terms []string) string {
	output := text
	for _, term := range terms {
		if term == "" {
			continue
		}
		output = strings.ReplaceAll(output, term, "**"+term+"**")
	}
	return output
}

func (p *Plugin) fileResultsSection(list *model.FileInfoList, terms string) (string, bool) {
	if list == nil || len(list.Order) == 0 {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "##### Files for `%s`\n", terms)
	count := 0
	for _, id := range list.Order {
		file := list.FileInfos[id]
		if file == nil {
			continue
		}
		count++
		channelText, _ := p.channelResultText(file.ChannelId)
		fmt.Fprintf(&b, "- [`%s`](/api/v4/files/%s) in %s\n", file.Name, file.Id, channelText)
	}
	if count == 0 {
		return "", false
	}
	return b.String(), true
}

func (p *Plugin) channelResultText(channelID string) (string, string) {
	channel, appErr := p.API.GetChannel(channelID)
	if appErr != nil || channel == nil {
		return "~unknown", ""
	}

	channelName := channelDisplayName(channel)
	if channel.TeamId == "" || channel.Name == "" {
		return fmt.Sprintf("~%s", channelName), ""
	}

	team, teamErr := p.API.GetTeam(channel.TeamId)
	if teamErr != nil || team == nil || team.Name == "" {
		return fmt.Sprintf("~%s", channelName), ""
	}

	return fmt.Sprintf("[~%s](/%s/channels/%s)", channelName, team.Name, channel.Name), team.Name
}

func channelDisplayName(channel *model.Channel) string {
	if channel == nil {
		return "unknown"
	}
	if channel.DisplayName != "" {
		return channel.DisplayName
	}
	return channel.Name
}
