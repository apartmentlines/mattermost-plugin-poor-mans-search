package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	logsPerPage     = 100
	timeStampFormat = "2006-01-02 15:04:05.000 Z07:00"
)

func logs(ctx context.Context, client *model.Client4, pluginID string) error {
	if err := checkJSONLogsSetting(ctx, client); err != nil {
		return err
	}

	logs, err := fetchLogs(ctx, client, 0, 500, pluginID, time.Unix(0, 0))
	if err != nil {
		return fmt.Errorf("failed to fetch log entries: %w", err)
	}

	if err := printLogEntries(logs); err != nil {
		return fmt.Errorf("failed to print log entries: %w", err)
	}

	return nil
}

func watchLogs(ctx context.Context, client *model.Client4, pluginID string) error {
	if err := checkJSONLogsSetting(ctx, client); err != nil {
		return err
	}

	now := time.Now()
	var oldestEntry string

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			var page int
			for {
				logs, err := fetchLogs(ctx, client, page, logsPerPage, pluginID, now)
				if err != nil {
					return fmt.Errorf("failed to fetch log entries: %w", err)
				}

				var allNew bool
				logs, oldestEntry, allNew = checkOldestEntry(logs, oldestEntry)
				if err := printLogEntries(logs); err != nil {
					return fmt.Errorf("failed to print log entries: %w", err)
				}
				if !allNew {
					break
				}
				page++
			}
		}
	}
}

func checkOldestEntry(logs []string, oldest string) ([]string, string, bool) {
	if len(logs) == 0 {
		return nil, oldest, false
	}

	newOldestEntry := logs[len(logs)-1]
	i := slices.Index(logs, oldest)
	switch i {
	case -1:
		return logs, newOldestEntry, true
	case len(logs) - 1:
		return nil, oldest, false
	default:
		return logs[i+1:], newOldestEntry, false
	}
}

func fetchLogs(ctx context.Context, client *model.Client4, page, perPage int, pluginID string, since time.Time) ([]string, error) {
	logs, _, err := client.GetLogs(ctx, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs from Mattermost: %w", err)
	}

	logs, err = filterLogEntries(logs, pluginID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to filter log entries: %w", err)
	}

	return logs, nil
}

func filterLogEntries(logs []string, pluginID string, since time.Time) ([]string, error) {
	type logEntry struct {
		PluginID  string `json:"plugin_id"`
		Timestamp string `json:"timestamp"`
	}

	var ret []string
	for _, entry := range logs {
		var le logEntry
		if err := json.Unmarshal([]byte(entry), &le); err != nil {
			return nil, fmt.Errorf("failed to unmarshal log entry into JSON: %w", err)
		}
		if le.PluginID != pluginID {
			continue
		}

		entryTime, err := time.Parse(timeStampFormat, le.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("unknown timestamp format: %w", err)
		}
		if entryTime.Before(since) {
			continue
		}

		ret = append(ret, strings.TrimPrefix(entry, "\n"))
	}

	return ret, nil
}

func printLogEntries(entries []string) error {
	for _, entry := range entries {
		if _, err := io.WriteString(os.Stdout, entry+"\n"); err != nil {
			return fmt.Errorf("failed to write log entry to stdout: %w", err)
		}
	}

	return nil
}

func checkJSONLogsSetting(ctx context.Context, client *model.Client4) error {
	cfg, _, err := client.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch config: %w", err)
	}
	if cfg.LogSettings.FileJson == nil || !*cfg.LogSettings.FileJson {
		return errors.New("JSON output for file logs is disabled. Enable LogSettings.FileJson in Mattermost")
	}

	return nil
}
