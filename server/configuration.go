package main

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

const defaultBatchSize = 10000
const defaultIndexDir = "data/poor-mans-search/bleve"
const defaultSearchResultDisplay = searchResultDisplayInline

const (
	searchResultDisplayInline  = "inline"
	searchResultDisplaySidebar = "sidebar"
)

type configuration struct {
	IndexDir            string `json:"indexdir"`
	BatchSize           int    `json:"batchsize"`
	SearchResultDisplay string `json:"searchresultdisplay"`
}

func (c *configuration) Clone() *configuration {
	clone := *c
	return &clone
}

func (c *configuration) setDefaults() {
	if c.IndexDir == "" {
		c.IndexDir = defaultIndexDir
	}
	if c.BatchSize == 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.SearchResultDisplay == "" {
		c.SearchResultDisplay = defaultSearchResultDisplay
	}
}

func (c *configuration) validate() error {
	if c.IndexDir == "" {
		return fmt.Errorf("IndexDir is required")
	}
	if c.BatchSize < 1 {
		return fmt.Errorf("BatchSize must be at least 1")
	}
	switch c.SearchResultDisplay {
	case searchResultDisplayInline, searchResultDisplaySidebar:
	default:
		return fmt.Errorf("SearchResultDisplay must be %q or %q", searchResultDisplayInline, searchResultDisplaySidebar)
	}
	return nil
}

func (c *configuration) searchEngineEqual(other *configuration) bool {
	if c == nil || other == nil {
		return c == other
	}
	return c.IndexDir == other.IndexDir && c.BatchSize == other.BatchSize
}

func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		cfg := &configuration{}
		cfg.setDefaults()
		return cfg
	}

	return p.configuration.Clone()
}

func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}
		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

func (p *Plugin) OnConfigurationChange() error {
	cfg := &configuration{}
	if err := p.API.LoadPluginConfiguration(cfg); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return err
	}

	old := p.getConfiguration()
	p.setConfiguration(cfg)

	if p.engine != nil && !old.searchEngineEqual(cfg) {
		if err := p.engine.UpdateConfig(cfg); err != nil {
			p.API.LogError("Failed to apply search configuration", "error", err.Error())
			return err
		}
		status := p.engine.Status()
		p.API.LogInfo("Poor Man's Search configuration applied", "active", status.Active, "index_dir", status.IndexDir, "post_docs", status.PostDocs, "file_docs", status.FileDocs)
	}

	return nil
}
