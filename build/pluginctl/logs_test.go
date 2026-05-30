package main

import (
	"fmt"
	"testing"
	"time"
)

func TestCheckOldestEntry(t *testing.T) {
	for name, tc := range map[string]struct {
		logs           []string
		oldest         string
		expectedLogs   []string
		expectedOldest string
		expectedAllNew bool
	}{
		"nil logs": {
			logs:           nil,
			oldest:         "oldest",
			expectedLogs:   nil,
			expectedOldest: "oldest",
			expectedAllNew: false,
		},
		"empty logs": {
			logs:           []string{},
			oldest:         "oldest",
			expectedLogs:   nil,
			expectedOldest: "oldest",
			expectedAllNew: false,
		},
		"no new entries": {
			logs:           []string{"old1", "old2"},
			oldest:         "old2",
			expectedLogs:   nil,
			expectedOldest: "old2",
			expectedAllNew: false,
		},
		"all new entries": {
			logs:           []string{"new1", "new2"},
			oldest:         "old",
			expectedLogs:   []string{"new1", "new2"},
			expectedOldest: "new2",
			expectedAllNew: true,
		},
		"some new entries": {
			logs:           []string{"old1", "old2", "new1", "new2"},
			oldest:         "old2",
			expectedLogs:   []string{"new1", "new2"},
			expectedOldest: "new2",
			expectedAllNew: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs, oldest, allNew := checkOldestEntry(tc.logs, tc.oldest)
			if allNew != tc.expectedAllNew {
				t.Fatalf("expected allNew %v, got %v", tc.expectedAllNew, allNew)
			}
			if oldest != tc.expectedOldest {
				t.Fatalf("expected oldest %q, got %q", tc.expectedOldest, oldest)
			}
			compareSlice(t, tc.expectedLogs, logs)
		})
	}
}

func TestFilterLogEntries(t *testing.T) {
	now := time.Now()

	for name, tc := range map[string]struct {
		logs         []string
		pluginID     string
		since        time.Time
		expectedLogs []string
		expectedErr  bool
	}{
		"empty slice": {
			logs:         []string{},
			expectedLogs: nil,
		},
		"bad JSON": {
			logs:        []string{`{"foo"`},
			expectedErr: true,
		},
		"unknown time format": {
			logs:        []string{`{"message":"foo", "plugin_id": "some.plugin.id", "timestamp": "2023-12-18 10:58:53"}`},
			pluginID:    "some.plugin.id",
			expectedErr: true,
		},
		"matching entry": {
			logs: []string{
				`{"message":"foo", "plugin_id": "some.plugin.id", "timestamp": "2023-12-18 10:58:53.091 +01:00"}`,
			},
			pluginID: "some.plugin.id",
			expectedLogs: []string{
				`{"message":"foo", "plugin_id": "some.plugin.id", "timestamp": "2023-12-18 10:58:53.091 +01:00"}`,
			},
		},
		"filters non-plugin entries": {
			logs: []string{
				`{"message":"bar1", "timestamp": "2023-12-18 10:58:52.091 +01:00"}`,
				`{"message":"foo", "plugin_id": "some.plugin.id", "timestamp": "2023-12-18 10:58:53.091 +01:00"}`,
			},
			pluginID: "some.plugin.id",
			expectedLogs: []string{
				`{"message":"foo", "plugin_id": "some.plugin.id", "timestamp": "2023-12-18 10:58:53.091 +01:00"}`,
			},
		},
		"filters old entries": {
			logs: []string{
				fmt.Sprintf(`{"message":"old", "plugin_id": "some.plugin.id", "timestamp": "%s"}`, now.Add(-time.Second).Format(timeStampFormat)),
				fmt.Sprintf(`{"message":"new", "plugin_id": "some.plugin.id", "timestamp": "%s"}`, now.Add(time.Second).Format(timeStampFormat)),
			},
			pluginID: "some.plugin.id",
			since:    now,
			expectedLogs: []string{
				fmt.Sprintf(`{"message":"new", "plugin_id": "some.plugin.id", "timestamp": "%s"}`, now.Add(time.Second).Format(timeStampFormat)),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs, err := filterLogEntries(tc.logs, tc.pluginID, tc.since)
			if tc.expectedErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			compareSlice(t, tc.expectedLogs, logs)
		})
	}
}

func compareSlice[S ~[]E, E comparable](t *testing.T, expected, got S) {
	if len(expected) != len(got) {
		t.Fatalf("expected len %d, got %d", len(expected), len(got))
	}
	for i := range expected {
		if expected[i] != got[i] {
			t.Fatalf("expected [%d] %v, got %v", i, expected[i], got[i])
		}
	}
}
