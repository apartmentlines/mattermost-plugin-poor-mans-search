# Configuration

Poor Man's Search has three plugin settings.

## Index Directory

`IndexDir` is the filesystem directory where Bleve indexes are stored. The plugin creates this directory recursively when it initializes the search engine.

Relative paths are resolved from the Mattermost working directory.

Default:

```text
data/poor-mans-search/bleve
```

The directory contains separate post and file indexes.

## /search Command Result Display

`SearchResultDisplay` controls how plugin-backed `/search` results are displayed.

- `inline`: `/search` slash commands are rewritten to the plugin-owned `/find` command. Results are shown as an ephemeral message in the current channel.
- `sidebar`: Mattermost's search sidebar displays plugin-backed results. This requires reverse proxy rewrites from Mattermost's native search API endpoints to the plugin API endpoints, see [Sidebar Results](sidebar-results.md).

Inline mode is the default and requires no reverse proxy setup.

Mattermost's search box still uses native Mattermost search in inline mode.

## Index Admin

`SearchAdmin` renders the plugin's System Console controls:

- **Index Now** starts a full rebuild from the Mattermost database.
- **Purge Index** removes plugin-owned Bleve indexes and clears rebuild history.
- The status table shows the running rebuild and recent completed rebuilds.

## Mattermost Config JSON

Plugin settings can also be configured in Mattermost's config file under `PluginSettings.Plugins`.

```json
{
  "PluginSettings": {
    "Plugins": {
      "com.mattermost.plugin-poor-mans-search": {
        "IndexDir": "data/poor-mans-search/bleve",
        "SearchResultDisplay": "inline"
      }
    }
  }
}
```

For sidebar results:

```json
{
  "PluginSettings": {
    "Plugins": {
      "com.mattermost.plugin-poor-mans-search": {
        "IndexDir": "data/poor-mans-search/bleve",
        "SearchResultDisplay": "sidebar"
      }
    }
  }
}
```

Sidebar mode also requires proxy configuration. See [Sidebar Results](sidebar-results.md).
