package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/mattermost/mattermost/server/public/model"
)

const postgresDriverName = model.DatabaseDriverPostgres

type backfillCursor struct {
	CreateAt int64
	ID       string
}

type postBackfillRow struct {
	Post   *model.Post
	TeamID string
}

type backfillStore interface {
	FetchPostBatch(ctx context.Context, cursor backfillCursor, limit int) ([]postBackfillRow, error)
	FetchFileBatch(ctx context.Context, cursor backfillCursor, limit int) ([]*model.FileInfo, error)
}

type postgresBackfillStore struct {
	db     *sql.DB
	schema postgresSchema
}

type postgresSchema interface {
	FetchPostBatch(ctx context.Context, db *sql.DB, cursor backfillCursor, limit int) ([]postBackfillRow, error)
	FetchFileBatch(ctx context.Context, db *sql.DB, cursor backfillCursor, limit int) ([]*model.FileInfo, error)
}

func newBackfillStore(db *sql.DB, driverName, serverVersion string) (backfillStore, error) {
	if driverName != postgresDriverName {
		return nil, fmt.Errorf("unsupported database driver %q; only %q is supported", driverName, postgresDriverName)
	}
	schema, err := selectPostgresSchema(serverVersion)
	if err != nil {
		return nil, err
	}
	return &postgresBackfillStore{db: db, schema: schema}, nil
}

type postgresSchemaV11 struct{}

func selectPostgresSchema(serverVersion string) (postgresSchema, error) {
	version := strings.TrimPrefix(serverVersion, "v")
	parsed, err := semver.ParseTolerant(version)
	if err != nil {
		return nil, fmt.Errorf("unsupported Mattermost version %q: %w", serverVersion, err)
	}

	// The plugin targets the post-Bleve-removal Mattermost schema. The removal
	// landed during the Mattermost 11 development line.
	if parsed.Major < 11 {
		return nil, fmt.Errorf("unsupported Mattermost version %q; expected 11.0.0 or newer", serverVersion)
	}
	return postgresSchemaV11{}, nil
}

func (s *postgresBackfillStore) FetchPostBatch(ctx context.Context, cursor backfillCursor, limit int) ([]postBackfillRow, error) {
	return s.schema.FetchPostBatch(ctx, s.db, cursor, limit)
}

func (s *postgresBackfillStore) FetchFileBatch(ctx context.Context, cursor backfillCursor, limit int) ([]*model.FileInfo, error) {
	return s.schema.FetchFileBatch(ctx, s.db, cursor, limit)
}

func (postgresSchemaV11) FetchPostBatch(ctx context.Context, db *sql.DB, cursor backfillCursor, limit int) ([]postBackfillRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT Posts.Id, Posts.ChannelId, Posts.UserId, Posts.CreateAt, Posts.DeleteAt,
		       Posts.Message, Posts.Type, Posts.Hashtags, Channels.TeamId
		FROM Posts
		LEFT JOIN Channels ON Posts.ChannelId = Channels.Id
		WHERE (Posts.CreateAt, Posts.Id) > ($1, $2)
		ORDER BY Posts.CreateAt ASC, Posts.Id ASC
		LIMIT $3`, cursor.CreateAt, cursor.ID, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var batch []postBackfillRow
	for rows.Next() {
		post := &model.Post{}
		var teamID sql.NullString
		if err := rows.Scan(&post.Id, &post.ChannelId, &post.UserId, &post.CreateAt, &post.DeleteAt, &post.Message, &post.Type, &post.Hashtags, &teamID); err != nil {
			return nil, err
		}
		batch = append(batch, postBackfillRow{
			Post:   post,
			TeamID: teamID.String,
		})
	}
	return batch, rows.Err()
}

func (postgresSchemaV11) FetchFileBatch(ctx context.Context, db *sql.DB, cursor backfillCursor, limit int) ([]*model.FileInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT Id, CreatorId, PostId, ChannelId, CreateAt, DeleteAt, Name, Extension, Content
		FROM FileInfo
		WHERE (CreateAt, Id) > ($1, $2)
		ORDER BY CreateAt ASC, Id ASC
		LIMIT $3`, cursor.CreateAt, cursor.ID, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var batch []*model.FileInfo
	for rows.Next() {
		var file model.FileInfo
		var content sql.NullString
		if err := rows.Scan(&file.Id, &file.CreatorId, &file.PostId, &file.ChannelId, &file.CreateAt, &file.DeleteAt, &file.Name, &file.Extension, &content); err != nil {
			return nil, err
		}
		if content.Valid {
			file.Content = content.String
		}
		batch = append(batch, &file)
	}
	return batch, rows.Err()
}
