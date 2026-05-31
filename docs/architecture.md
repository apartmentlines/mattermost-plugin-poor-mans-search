# Architecture

Poor Man's Search owns separate Bleve indexes for Mattermost posts and files and exposes those indexes through slash commands, plugin API endpoints, and System Console controls.

## Main Components

- **Search engine**: opens and maintains `posts.bleve` and `files.bleve` under the configured index directory.
- **Live indexing hooks**: index newly created posts and file metadata through Mattermost plugin hooks.
- **Backfill indexer**: rebuilds indexes from PostgreSQL using a Mattermost-version-aware schema adapter.
- **Slash command path**: rewrites `/search` to the hidden plugin-owned `/find` command in inline mode.
- **Sidebar API path**: serves Mattermost-compatible post and file search responses for reverse-proxied native search requests.
- **Admin UI**: starts rebuilds, purges indexes, and displays rebuild status.

## Inline Results

Inline mode is the default path. The webapp plugin registers a slash-command hook that rewrites:

```text
/search terms
```

to:

```text
/find terms
```

The server-side `/find` command searches both post and file indexes and returns an ephemeral message in the current channel. The `/find` command is intentionally hidden from autocomplete.

Mattermost's search box is not intercepted in inline mode. Mattermost only exposes plugin search-box extensions to licensed servers, so free-tier installs cannot use that API.

## Sidebar Results

[Sidebar Results](sidebar-results.md) is for admins who want Mattermost's native search sidebar to display plugin-backed results. In this mode, a reverse proxy rewrites native Mattermost search API requests to plugin API endpoints.

The plugin serves:

```text
POST /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/{team_id}/posts/search
POST /plugins/com.mattermost.plugin-poor-mans-search/api/v1/posts/search
POST /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/{team_id}/files/search
POST /plugins/com.mattermost.plugin-poor-mans-search/api/v1/files/search
```

The user manually configures the core Mattermost search API paths to proxy to the plugin paths.

Post responses use Mattermost's `model.PostSearchResults`. File responses use `model.FileInfoList`.

## Permissions

Search results are filtered through Mattermost plugin APIs before being returned. The index may contain documents the user cannot see, but the visible result set is checked against the user's channel and post/file access.

## Rebuilds

The plugin has two indexing paths. Live indexing uses Mattermost hooks and public plugin APIs to index new posts and file attachments after the plugin is installed. Full rebuilds backfill historical data from the Mattermost database into the plugin-owned Bleve indexes.

An initial rebuild is required before existing Mattermost messages and files are searchable through Poor Man's Search. Without it, only content created after the plugin starts can be indexed automatically.

Full rebuilds are narrower by design:

- PostgreSQL only.
- Mattermost server version is used to select a known schema adapter.
- Unsupported server versions or database drivers fail before the rebuild starts.

Rebuilds can run while search is available, but results may be incomplete until the rebuild finishes. Rebuilds are admin-triggered backfill and repair operations, not a periodic sync loop.

## Purging

Purging removes plugin-owned Bleve index contents and clears rebuild history. Search results remain incomplete until another rebuild has repopulated the indexes.
