package main

import (
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type Plugin struct {
	plugin.MattermostPlugin

	configurationLock sync.RWMutex
	configuration     *configuration

	client  *pluginapi.Client
	engine  *searchEngine
	indexer *indexer
}

func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)
	if err := p.OnConfigurationChange(); err != nil {
		return err
	}
	p.engine = newSearchEngine(p.getConfiguration())
	if err := p.engine.Start(); err != nil {
		return err
	}
	status := p.engine.Status()
	p.API.LogInfo("Poor Man's Search engine initialized", "active", status.Active, "index_dir", status.IndexDir, "post_docs", status.PostDocs, "file_docs", status.FileDocs)
	p.indexer = newIndexer(p)
	return p.registerFindCommand()
}

func (p *Plugin) OnDeactivate() error {
	if p.indexer != nil {
		p.indexer.Stop()
	}
	if p.engine != nil {
		return p.engine.Stop()
	}
	return nil
}

func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, post *model.Post) {
	p.indexPostAndFiles(post)
}

func (p *Plugin) MessageHasBeenUpdated(_ *plugin.Context, newPost, _ *model.Post) {
	p.indexPostAndFiles(newPost)
}

func (p *Plugin) MessageHasBeenDeleted(_ *plugin.Context, post *model.Post) {
	if p.engine == nil {
		return
	}
	if err := p.engine.DeletePost(post.Id); err != nil {
		p.API.LogWarn("Failed to delete post from search index", "post_id", post.Id, "error", err.Error())
	}
	for _, fileID := range post.FileIds {
		if err := p.engine.IndexFile(&model.FileInfo{Id: fileID, DeleteAt: 1}); err != nil {
			p.API.LogWarn("Failed to delete file from search index", "file_id", fileID, "error", err.Error())
		}
	}
}

func (p *Plugin) indexPostAndFiles(post *model.Post) {
	if p.engine == nil || !p.engine.Active() {
		return
	}
	channel, appErr := p.API.GetChannel(post.ChannelId)
	if appErr != nil {
		p.API.LogWarn("Failed to load channel for search indexing", "post_id", post.Id, "channel_id", post.ChannelId, "error", appErr.Error())
		return
	}
	if err := p.engine.IndexPost(post, channel.TeamId); err != nil {
		p.API.LogWarn("Failed to index post", "post_id", post.Id, "error", err.Error())
	}
	for _, fileID := range post.FileIds {
		file, appErr := p.API.GetFileInfo(fileID)
		if appErr != nil {
			p.API.LogWarn("Failed to load file for search indexing", "post_id", post.Id, "file_id", fileID, "error", appErr.Error())
			continue
		}
		if file.ChannelId == "" {
			file.ChannelId = post.ChannelId
		}
		if file.PostId == "" {
			file.PostId = post.Id
		}
		if err := p.engine.IndexFile(file); err != nil {
			p.API.LogWarn("Failed to index file", "file_id", file.Id, "error", err.Error())
		}
	}
}
