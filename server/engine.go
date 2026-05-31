package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/mattermost/mattermost/server/public/model"
)

const (
	postIndexName = "posts"
	fileIndexName = "files"
)

type blvPost struct {
	ID        string
	TeamID    string
	ChannelID string
	UserID    string
	CreateAt  int64
	Message   string
	Type      string
	Hashtags  []string
}

type blvFile struct {
	ID        string
	PostID    string
	CreatorID string
	ChannelID string
	CreateAt  int64
	Name      string
	Content   string
	Extension string
}

type searchEngine struct {
	mut       sync.RWMutex
	ready     int32
	cfg       *configuration
	postIndex bleve.Index
	fileIndex bleve.Index
}

type engineStatus struct {
	Active        bool   `json:"active"`
	IndexDir      string `json:"index_dir,omitempty"`
	PostIndexPath string `json:"post_index_path,omitempty"`
	FileIndexPath string `json:"file_index_path,omitempty"`
	PostDocs      uint64 `json:"post_docs"`
	FileDocs      uint64 `json:"file_docs"`
	PostCountErr  string `json:"post_count_error,omitempty"`
	FileCountErr  string `json:"file_count_error,omitempty"`
}

type searchHit struct {
	ID      string
	Matches []string
}

func newSearchEngine(cfg *configuration) *searchEngine {
	return &searchEngine{cfg: cfg.Clone()}
}

func (e *searchEngine) Start() error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.openLocked()
}

func (e *searchEngine) Stop() error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.closeLocked()
}

func (e *searchEngine) UpdateConfig(cfg *configuration) error {
	e.mut.Lock()
	defer e.mut.Unlock()

	needsReopen := e.cfg == nil ||
		e.cfg.IndexDir != cfg.IndexDir

	if needsReopen {
		if err := e.closeLocked(); err != nil {
			return err
		}
		e.cfg = cfg.Clone()
		return e.openLocked()
	}

	e.cfg = cfg.Clone()
	return nil
}

func (e *searchEngine) Active() bool {
	return atomic.LoadInt32(&e.ready) == 1
}

func (e *searchEngine) Status() engineStatus {
	e.mut.RLock()
	defer e.mut.RUnlock()

	status := engineStatus{Active: e.Active()}
	if e.cfg != nil {
		status.IndexDir = e.cfg.IndexDir
		if e.cfg.IndexDir != "" {
			status.PostIndexPath = e.indexDir(postIndexName)
			status.FileIndexPath = e.indexDir(fileIndexName)
		}
	}
	if e.postIndex != nil {
		count, err := e.postIndex.DocCount()
		if err != nil {
			status.PostCountErr = err.Error()
		} else {
			status.PostDocs = count
		}
	}
	if e.fileIndex != nil {
		count, err := e.fileIndex.DocCount()
		if err != nil {
			status.FileCountErr = err.Error()
		} else {
			status.FileDocs = count
		}
	}

	return status
}

func (e *searchEngine) openLocked() error {
	if e.cfg == nil || e.cfg.IndexDir == "" {
		return nil
	}
	if e.Active() {
		return nil
	}
	if err := os.MkdirAll(e.cfg.IndexDir, 0o750); err != nil {
		return err
	}

	var err error
	e.postIndex, err = createOrOpenIndex(e.indexDir(postIndexName), postIndexMapping())
	if err != nil {
		return fmt.Errorf("open post index: %w", err)
	}
	e.fileIndex, err = createOrOpenIndex(e.indexDir(fileIndexName), fileIndexMapping())
	if err != nil {
		_ = e.postIndex.Close()
		e.postIndex = nil
		return fmt.Errorf("open file index: %w", err)
	}

	atomic.StoreInt32(&e.ready, 1)
	return nil
}

func (e *searchEngine) closeLocked() error {
	var closeErr error
	if e.postIndex != nil {
		if err := e.postIndex.Close(); err != nil {
			closeErr = err
		}
		e.postIndex = nil
	}
	if e.fileIndex != nil {
		if err := e.fileIndex.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		e.fileIndex = nil
	}
	atomic.StoreInt32(&e.ready, 0)
	return closeErr
}

func (e *searchEngine) Purge() error {
	e.mut.Lock()
	defer e.mut.Unlock()

	if e.cfg == nil || e.cfg.IndexDir == "" {
		return nil
	}
	if err := e.closeLocked(); err != nil {
		return err
	}
	if err := os.RemoveAll(e.indexDir(postIndexName)); err != nil {
		return err
	}
	if err := os.RemoveAll(e.indexDir(fileIndexName)); err != nil {
		return err
	}
	return e.openLocked()
}

func (e *searchEngine) indexDir(indexName string) string {
	return filepath.Join(e.cfg.IndexDir, indexName+".bleve")
}

func createOrOpenIndex(indexPath string, indexMapping *mapping.IndexMappingImpl) (bleve.Index, error) {
	if index, err := bleve.Open(indexPath); err == nil {
		return index, nil
	}
	return bleve.NewUsing(indexPath, indexMapping, "scorch", "scorch", map[string]any{
		"forceSegmentType":    "zap",
		"forceSegmentVersion": 15,
	})
}

func postIndexMapping() *mapping.IndexMappingImpl {
	doc := bleve.NewDocumentMapping()
	doc.AddFieldMappingsAt("ID", keywordField())
	doc.AddFieldMappingsAt("TeamID", keywordField())
	doc.AddFieldMappingsAt("ChannelID", keywordField())
	doc.AddFieldMappingsAt("UserID", keywordField())
	doc.AddFieldMappingsAt("CreateAt", bleve.NewNumericFieldMapping())
	doc.AddFieldMappingsAt("Message", standardField())
	doc.AddFieldMappingsAt("Type", keywordField())
	doc.AddFieldMappingsAt("Hashtags", standardField())
	idx := bleve.NewIndexMapping()
	idx.AddDocumentMapping("_default", doc)
	return idx
}

func fileIndexMapping() *mapping.IndexMappingImpl {
	doc := bleve.NewDocumentMapping()
	doc.AddFieldMappingsAt("ID", keywordField())
	doc.AddFieldMappingsAt("PostID", keywordField())
	doc.AddFieldMappingsAt("CreatorID", keywordField())
	doc.AddFieldMappingsAt("ChannelID", keywordField())
	doc.AddFieldMappingsAt("CreateAt", bleve.NewNumericFieldMapping())
	doc.AddFieldMappingsAt("Name", standardField())
	doc.AddFieldMappingsAt("Content", standardField())
	doc.AddFieldMappingsAt("Extension", keywordField())
	idx := bleve.NewIndexMapping()
	idx.AddDocumentMapping("_default", doc)
	return idx
}

func keywordField() *mapping.FieldMapping {
	f := bleve.NewTextFieldMapping()
	f.Analyzer = keyword.Name
	return f
}

func standardField() *mapping.FieldMapping {
	f := bleve.NewTextFieldMapping()
	f.Analyzer = standard.Name
	return f
}

func postToBLV(post *model.Post, teamID string) *blvPost {
	return &blvPost{
		ID:        post.Id,
		TeamID:    teamID,
		ChannelID: post.ChannelId,
		UserID:    post.UserId,
		CreateAt:  post.CreateAt,
		Message:   post.Message,
		Type:      post.Type,
		Hashtags:  strings.Fields(post.Hashtags),
	}
}

func fileToBLV(file *model.FileInfo) *blvFile {
	return &blvFile{
		ID:        file.Id,
		PostID:    file.PostId,
		ChannelID: file.ChannelId,
		CreatorID: file.CreatorId,
		CreateAt:  file.CreateAt,
		Content:   file.Content,
		Extension: file.Extension,
		Name:      file.Name + " " + splitFilenameWords(file.Name),
	}
}

func splitFilenameWords(name string) string {
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, ".", " ")
	return name
}

func (e *searchEngine) IndexPost(post *model.Post, teamID string) error {
	if !e.Active() {
		return nil
	}
	e.mut.RLock()
	defer e.mut.RUnlock()
	if post.DeleteAt != 0 {
		return e.postIndex.Delete(post.Id)
	}
	return e.postIndex.Index(post.Id, postToBLV(post, teamID))
}

func (e *searchEngine) DeletePost(postID string) error {
	if !e.Active() {
		return nil
	}
	e.mut.RLock()
	defer e.mut.RUnlock()
	return e.postIndex.Delete(postID)
}

func (e *searchEngine) IndexFile(file *model.FileInfo) error {
	if !e.Active() {
		return nil
	}
	e.mut.RLock()
	defer e.mut.RUnlock()
	if file.DeleteAt != 0 || file.PostId == "" {
		return e.fileIndex.Delete(file.Id)
	}
	return e.fileIndex.Index(file.Id, fileToBLV(file))
}

func (e *searchEngine) SearchPosts(channels model.ChannelList, searchParams []*model.SearchParams, offset, limit int) ([]searchHit, error) {
	if !e.Active() {
		return nil, fmt.Errorf("search index is not active")
	}
	if len(channels) == 0 || len(searchParams) == 0 {
		return []searchHit{}, nil
	}
	q := buildPostQuery(channels, searchParams)
	req := bleve.NewSearchRequestOptions(q, limit, offset, false)
	req.Highlight = bleve.NewHighlight()
	req.Highlight.AddField("Message")
	req.SortBy([]string{"-CreateAt"})

	e.mut.RLock()
	defer e.mut.RUnlock()
	results, err := e.postIndex.Search(req)
	if err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(results.Hits))
	for _, hit := range results.Hits {
		hits = append(hits, searchHit{
			ID:      hit.ID,
			Matches: highlightedTerms(hit.Fragments["Message"], "mark"),
		})
	}
	return hits, nil
}

func highlightedTerms(fragments []string, tag string) []string {
	seen := map[string]bool{}
	var terms []string

	for _, fragment := range fragments {
		decoder := xml.NewDecoder(strings.NewReader(fragment))
		inMatch := false

		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}

			switch typed := token.(type) {
			case xml.StartElement:
				if typed.Name.Local == tag {
					inMatch = true
				}
			case xml.EndElement:
				if typed.Name.Local == tag {
					inMatch = false
				}
			case xml.CharData:
				if !inMatch || len(typed) == 0 {
					continue
				}
				term := strings.Trim(string(typed), "_*~")
				if term != "" && !seen[term] {
					seen[term] = true
					terms = append(terms, term)
				}
			}
		}
	}

	return terms
}

func (e *searchEngine) SearchFiles(channels model.ChannelList, searchParams []*model.SearchParams, offset, limit int) ([]string, error) {
	if !e.Active() {
		return nil, fmt.Errorf("search index is not active")
	}
	if len(channels) == 0 || len(searchParams) == 0 {
		return []string{}, nil
	}
	q := buildFileQuery(channels, searchParams)
	req := bleve.NewSearchRequestOptions(q, limit, offset, false)
	req.SortBy([]string{"-CreateAt"})

	e.mut.RLock()
	defer e.mut.RUnlock()
	results, err := e.fileIndex.Search(req)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(results.Hits))
	for _, hit := range results.Hits {
		ids = append(ids, hit.ID)
	}
	return ids, nil
}

func channelQuery(channels model.ChannelList) query.Query {
	queries := make([]query.Query, 0, len(channels))
	for _, channel := range channels {
		channelQ := bleve.NewTermQuery(channel.Id)
		channelQ.SetField("ChannelID")
		queries = append(queries, channelQ)
	}
	return bleve.NewDisjunctionQuery(queries...)
}
