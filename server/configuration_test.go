package main

import (
	"strings"
	"testing"
)

func TestConfigurationDefaults(t *testing.T) {
	cfg := &configuration{}
	cfg.setDefaults()

	if cfg.BatchSize != defaultBatchSize {
		t.Fatalf("expected default batch size %d, got %d", defaultBatchSize, cfg.BatchSize)
	}
	if cfg.IndexDir != defaultIndexDir {
		t.Fatalf("expected default index dir %q, got %q", defaultIndexDir, cfg.IndexDir)
	}
	if cfg.SearchResultDisplay != defaultSearchResultDisplay {
		t.Fatalf("expected default search result display %q, got %q", defaultSearchResultDisplay, cfg.SearchResultDisplay)
	}
}

func TestConfigurationValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     configuration
		wantErr string
	}{
		{
			name: "index dir required",
			cfg: configuration{
				BatchSize: 1,
			},
			wantErr: "IndexDir",
		},
		{
			name: "batch size must be positive",
			cfg: configuration{
				IndexDir:  "/tmp/index",
				BatchSize: 0,
			},
			wantErr: "BatchSize",
		},
		{
			name: "valid",
			cfg: configuration{
				IndexDir:            "/tmp/index",
				BatchSize:           1,
				SearchResultDisplay: searchResultDisplayInline,
			},
		},
		{
			name: "valid sidebar",
			cfg: configuration{
				IndexDir:            "/tmp/index",
				BatchSize:           1,
				SearchResultDisplay: searchResultDisplaySidebar,
			},
		},
		{
			name: "invalid search result display",
			cfg: configuration{
				IndexDir:            "/tmp/index",
				BatchSize:           1,
				SearchResultDisplay: "popup",
			},
			wantErr: "SearchResultDisplay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
