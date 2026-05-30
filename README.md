# Poor Man's Search

Poor Man's Search restores Bleve-backed message and file search for Mattermost 11.

## Requirements

- Mattermost 11.0.0 or newer with plugins enabled.
- PostgreSQL database backend.

## Quickstart

1. Build the plugin bundle:

```sh
make bundle
```

2. Upload and enable `dist/com.mattermost.plugin-poor-mans-search-[release_number]-dev.tar.gz` in Mattermost.

3. Open **System Console > Plugin Poor Man's Search**.

4. Set a custom `Index directory` if needed, default is `[mattermost_working_directory]/data/poor-mans-search/bleve`

4. Leave `/search command result display` set to `Inline results`.

5. Click **Index Now** and wait for the rebuild to finish.

6. Search using the `/search` slash command from any channel:

```text
/search your search terms
```

Inline mode rewrites `/search` slash commands as an ephemeral message in the current channel.

## Search Box

While inline mode is the simplest to set up, it does **not** handle searches in the top search box -- that still uses Mattermost's native search in inline mode.

To make the normal search box/sidebar use Poor Man's Search, configure [Sidebar Results](docs/sidebar-results.md).

## More Documentation

- [Configuration](docs/configuration.md)
- [Sidebar Results](docs/sidebar-results.md)
- [Development](docs/development.md)
- [Architecture](docs/architecture.md)
