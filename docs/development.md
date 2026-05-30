# Development

## Build

Run tests and build a plugin bundle:

```sh
make test
make bundle
```

The bundle is written to:

```text
dist/com.mattermost.plugin-poor-mans-search-[release_version]-dev.tar.gz
```

`make dist` builds all server executables declared in `plugin.json` and packages the plugin. The webapp bundle is intentionally simple: `make webapp` copies `webapp/src/plugin.js` to `webapp/dist/main.js`.

## Local Deploy

For local mode deploys, enable Mattermost local mode and plugin uploads:

```json
{
  "PluginSettings": {
    "EnableUploads": true
  },
  "ServiceSettings": {
    "EnableLocalMode": true,
    "LocalModeSocketLocation": "/var/tmp/mattermost_local.socket"
  }
}
```

Then deploy:

```sh
make deploy
```

You can override the socket path with `MM_LOCALSOCKETPATH`.

If local mode is unavailable, `make deploy` falls back to the API client and requires:

- `MM_SERVICESETTINGS_SITEURL`
- either `MM_ADMIN_TOKEN` or `MM_ADMIN_USERNAME` and `MM_ADMIN_PASSWORD`

## Useful Make Targets

```sh
make help
make vtest
make check-style
make deploy
make reset
make logs
make logs-watch
```

## Debugging

Build with debug flags:

```sh
MM_DEBUG=1 make deploy
```

Attach Delve to a running plugin:

```sh
make attach
```

Start headless Delve:

```sh
make attach-headless
```

The default headless Delve port is `2346`; override it with `DLV_DEBUG_PORT`.

## Development Proxy

The repository root includes a `Caddyfile` for local sidebar-mode testing. It listens on `http://localhost:8064`, forwards to Mattermost on `http://localhost:8065`, and rewrites native search API calls to the plugin API.

Run it with:

```sh
caddy run --config Caddyfile
```

While using that proxy, set Mattermost's `ServiceSettings.SiteURL` to `http://localhost:8064`.
