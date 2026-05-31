// Package main implements the Poor Man's Search Mattermost plugin.
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const mattermostUserIDHeader = "Mattermost-User-Id"

func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()
	api.Use(p.requireMattermostUser)
	api.HandleFunc("/teams/{team_id}/posts/search", p.handlePostSearch).Methods(http.MethodPost)
	api.HandleFunc("/posts/search", p.handlePostSearch).Methods(http.MethodPost)
	api.HandleFunc("/teams/{team_id}/files/search", p.handleFileSearch).Methods(http.MethodPost)
	api.HandleFunc("/files/search", p.handleFileSearch).Methods(http.MethodPost)
	api.HandleFunc("/index/rebuild", p.handleRebuildIndex).Methods(http.MethodPost)
	api.HandleFunc("/index/purge", p.handlePurgeIndex).Methods(http.MethodPost)
	api.HandleFunc("/index/status", p.handleIndexStatus).Methods(http.MethodGet)
	api.HandleFunc("/config/client", p.handleClientConfig).Methods(http.MethodGet)
	router.ServeHTTP(w, r)
}

func (p *Plugin) requireMattermostUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(mattermostUserIDHeader) == "" {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) handlePostSearch(w http.ResponseWriter, r *http.Request) {
	var params model.SearchParameter
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		p.API.LogWarn("Invalid post search API request body", "path", r.URL.Path, "error", err.Error())
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	start := time.Now()
	userID := r.Header.Get(mattermostUserIDHeader)
	teamID := mux.Vars(r)["team_id"]
	p.logSearchAPIRequest("post", r.URL.Path, userID, teamID, params)
	results, err := p.runPostSearch(userID, teamID, params)
	if err != nil {
		p.API.LogWarn("Post search API request failed", "path", r.URL.Path, "user_id", userID, "team_id", teamID, "error", err.Error(), "elapsed_ms", time.Since(start).Milliseconds())
		p.writeSearchError(w, err)
		return
	}
	p.API.LogDebug("Post search API request completed", "path", r.URL.Path, "user_id", userID, "team_id", teamID, "results", len(results.Posts), "has_next", model.SafeDereference(results.HasNext), "elapsed_ms", time.Since(start).Milliseconds())
	writeJSON(w, results)
}

func (p *Plugin) handleFileSearch(w http.ResponseWriter, r *http.Request) {
	var params model.SearchParameter
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		p.API.LogWarn("Invalid file search API request body", "path", r.URL.Path, "error", err.Error())
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	start := time.Now()
	userID := r.Header.Get(mattermostUserIDHeader)
	teamID := mux.Vars(r)["team_id"]
	p.logSearchAPIRequest("file", r.URL.Path, userID, teamID, params)
	results, err := p.runFileSearch(userID, teamID, params)
	if err != nil {
		p.API.LogWarn("File search API request failed", "path", r.URL.Path, "user_id", userID, "team_id", teamID, "error", err.Error(), "elapsed_ms", time.Since(start).Milliseconds())
		p.writeSearchError(w, err)
		return
	}
	p.API.LogDebug("File search API request completed", "path", r.URL.Path, "user_id", userID, "team_id", teamID, "results", len(results.FileInfos), "elapsed_ms", time.Since(start).Milliseconds())
	writeJSON(w, results)
}

func (p *Plugin) logSearchAPIRequest(kind, path, userID, teamID string, params model.SearchParameter) {
	fields := []any{
		"kind", kind,
		"path", path,
		"user_id", userID,
		"team_id", teamID,
		"terms_len", len(model.SafeDereference(params.Terms)),
		"is_or_search", model.SafeDereference(params.IsOrSearch),
		"page", model.SafeDereference(params.Page),
		"per_page", model.SafeDereference(params.PerPage),
		"include_deleted_channels", model.SafeDereference(params.IncludeDeletedChannels),
	}
	p.API.LogDebug("Search API request received", fields...)
}

func (p *Plugin) handleRebuildIndex(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(mattermostUserIDHeader)
	if !p.API.HasPermissionTo(userID, model.PermissionManageSystem) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := p.indexer.StartRebuild(); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]string{"status": "started"})
}

func (p *Plugin) handlePurgeIndex(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(mattermostUserIDHeader)
	if !p.API.HasPermissionTo(userID, model.PermissionManageSystem) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if status := p.indexer.Status(); status.Running {
		http.Error(w, "cannot purge while an index rebuild is running", http.StatusConflict)
		return
	}
	before := p.engine.Status()
	p.API.LogInfo("Purging search indexes", "post_docs", before.PostDocs, "file_docs", before.FileDocs, "index_dir", before.IndexDir)
	if err := p.engine.Purge(); err != nil {
		p.API.LogError("Failed to purge search indexes", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := p.indexer.ClearHistory(); err != nil {
		p.API.LogWarn("Failed to clear search index rebuild history after purge", "error", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.indexer.ResetStatus()
	after := p.engine.Status()
	p.API.LogInfo("Search indexes purged", "post_docs", after.PostDocs, "file_docs", after.FileDocs, "index_dir", after.IndexDir)
	writeJSON(w, map[string]any{
		"status": "purged",
		"engine": after,
	})
}

func (p *Plugin) handleIndexStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(mattermostUserIDHeader)
	if !p.API.HasPermissionTo(userID, model.PermissionManageSystem) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	history, err := p.indexer.History()
	if err != nil {
		p.API.LogWarn("Failed to load search index rebuild history", "error", err.Error())
		history = []indexHistoryEntry{}
	}
	writeJSON(w, map[string]any{
		"engine":  p.engine.Status(),
		"rebuild": p.indexer.Status(),
		"history": history,
	})
}

func (p *Plugin) handleClientConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := p.getConfiguration()
	writeJSON(w, map[string]string{
		"search_result_display": cfg.SearchResultDisplay,
	})
}

func (p *Plugin) writeSearchError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*model.AppError); ok {
		http.Error(w, appErr.Error(), appErr.StatusCode)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_ = json.NewEncoder(w).Encode(v)
}
