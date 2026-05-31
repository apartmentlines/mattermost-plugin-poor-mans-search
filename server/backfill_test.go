package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSelectPostgresSchema(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{name: "v prefix", version: "v11.0.0"},
		{name: "patch prerelease tolerant", version: "11.1.0-dev"},
		{name: "unsupported old version", version: "10.12.0", wantErr: true},
		{name: "invalid version", version: "not-a-version", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := selectPostgresSchema(tt.version)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !tt.wantErr {
				if _, ok := schema.(postgresSchemaV11); !ok {
					t.Fatalf("expected postgresSchemaV11, got %T", schema)
				}
			}
		})
	}
}

func TestNewBackfillStoreRejectsNonPostgres(t *testing.T) {
	_, err := newBackfillStore(nil, "mysql", "11.0.0")
	if err == nil {
		t.Fatal("expected unsupported driver error")
	}
}

func TestPostgresSchemaV11FetchPostBatchScansRows(t *testing.T) {
	db, mock := newSQLMock(t)
	rows := sqlmock.NewRows([]string{"Id", "ChannelId", "UserId", "CreateAt", "DeleteAt", "Message", "Type", "Hashtags", "TeamId"}).
		AddRow("post1", "channel1", "user1", int64(100), int64(0), "hello", "", "#tag", "team1").
		AddRow("post2", "channel2", "user2", int64(200), int64(1), "deleted", "", "", nil)
	mock.ExpectQuery("SELECT Posts\\.Id").
		WithArgs(int64(50), "cursor", 2).
		WillReturnRows(rows)

	batch, err := (postgresSchemaV11{}).FetchPostBatch(context.Background(), db, backfillCursor{CreateAt: 50, ID: "cursor"}, 2)
	if err != nil {
		t.Fatalf("fetch post batch: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 posts, got %#v", batch)
	}
	if batch[0].Post.Id != "post1" || batch[0].TeamID != "team1" || batch[0].Post.Hashtags != "#tag" {
		t.Fatalf("unexpected first row: %#v", batch[0])
	}
	if batch[1].Post.DeleteAt != 1 || batch[1].TeamID != "" {
		t.Fatalf("expected deleted post with empty team id, got %#v", batch[1])
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresSchemaV11FetchFileBatchScansRows(t *testing.T) {
	db, mock := newSQLMock(t)
	rows := sqlmock.NewRows([]string{"Id", "CreatorId", "PostId", "ChannelId", "CreateAt", "DeleteAt", "Name", "Extension", "Content"}).
		AddRow("file1", "user1", "post1", "channel1", int64(100), int64(0), "packet.pdf", "pdf", "welcome packet").
		AddRow("file2", "user2", "post2", "", int64(200), int64(0), "empty.txt", "txt", nil)
	mock.ExpectQuery("SELECT Id, CreatorId, PostId, COALESCE\\(ChannelId, ''\\) AS ChannelId").
		WithArgs(int64(50), "cursor", 2).
		WillReturnRows(rows)

	batch, err := (postgresSchemaV11{}).FetchFileBatch(context.Background(), db, backfillCursor{CreateAt: 50, ID: "cursor"}, 2)
	if err != nil {
		t.Fatalf("fetch file batch: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 files, got %#v", batch)
	}
	if batch[0].Id != "file1" || batch[0].Content != "welcome packet" || batch[0].Extension != "pdf" {
		t.Fatalf("unexpected first file: %#v", batch[0])
	}
	if batch[1].Id != "file2" || batch[1].ChannelId != "" || batch[1].Content != "" {
		t.Fatalf("expected nil channel/content to become empty strings, got %#v", batch[1])
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresSchemaV11FetchBatchErrors(t *testing.T) {
	db, mock := newSQLMock(t)
	queryErr := errors.New("query failed")
	mock.ExpectQuery("SELECT Posts\\.Id").WillReturnError(queryErr)
	if _, err := (postgresSchemaV11{}).FetchPostBatch(context.Background(), db, backfillCursor{}, 1); !errors.Is(err, queryErr) {
		t.Fatalf("expected query error, got %v", err)
	}

	rows := sqlmock.NewRows([]string{"Id", "CreatorId", "PostId", "ChannelId", "CreateAt", "DeleteAt", "Name", "Extension", "Content"}).
		AddRow("file1", "user1", "post1", "channel1", "not-an-int", int64(0), "packet.pdf", "pdf", "content")
	mock.ExpectQuery("SELECT Id, CreatorId, PostId").WillReturnRows(rows)
	if _, err := (postgresSchemaV11{}).FetchFileBatch(context.Background(), db, backfillCursor{}, 1); err == nil {
		t.Fatal("expected scan error")
	}
	assertSQLExpectations(t, mock)
}

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("new sql mock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db, mock
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
