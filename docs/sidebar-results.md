# Sidebar Results

Sidebar mode makes Mattermost's normal search sidebar display Poor Man's Search results. It works by reverse proxying Mattermost's native search API requests to the plugin's search API endpoints.

Use sidebar mode when you want the normal search box and right-hand search results pane to use plugin-backed Bleve search.

## Plugin Setting

Set `/search command result display` to `Sidebar results`, or configure:

```json
{
  "PluginSettings": {
    "Plugins": {
      "com.mattermost.plugin-poor-mans-search": {
        "searchresultdisplay": "sidebar"
      }
    }
  }
}
```

## Proxy Rules

The examples below use:

- Public Mattermost URL: `https://mattermost.example.com`
- Mattermost upstream server: `http://localhost:8065`

The proxy *must* rewrite these native API endpoints...

- `/api/v4/teams/{team_id}/posts/search`
- `/api/v4/posts/search`
- `/api/v4/teams/{team_id}/files/search`
- `/api/v4/files/search`

...to these plugin API endpoints:

- `/plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/{team_id}/posts/search`
- `/plugins/com.mattermost.plugin-poor-mans-search/api/v1/posts/search`
- `/plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/{team_id}/files/search`
- `/plugins/com.mattermost.plugin-poor-mans-search/api/v1/files/search`

All other requests should pass through to Mattermost unchanged.

## Caddy

```caddyfile
https://mattermost.example.com {
	@team_post_search {
		method POST
		path_regexp team_post_search ^/api/v4/teams/([^/]+)/posts/search$
	}
	handle @team_post_search {
		rewrite * /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/{re.team_post_search.1}/posts/search
		reverse_proxy http://localhost:8065
	}

	@post_search {
		method POST
		path /api/v4/posts/search
	}
	handle @post_search {
		rewrite * /plugins/com.mattermost.plugin-poor-mans-search/api/v1/posts/search
		reverse_proxy http://localhost:8065
	}

	@team_file_search {
		method POST
		path_regexp team_file_search ^/api/v4/teams/([^/]+)/files/search$
	}
	handle @team_file_search {
		rewrite * /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/{re.team_file_search.1}/files/search
		reverse_proxy http://localhost:8065
	}

	@file_search {
		method POST
		path /api/v4/files/search
	}
	handle @file_search {
		rewrite * /plugins/com.mattermost.plugin-poor-mans-search/api/v1/files/search
		reverse_proxy http://localhost:8065
	}

	handle {
		reverse_proxy http://localhost:8065
	}
}
```

## Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name mattermost.example.com;

    location ~ ^/api/v4/teams/([^/]+)/posts/search$ {
        rewrite ^/api/v4/teams/([^/]+)/posts/search$ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/$1/posts/search break;
        proxy_pass http://localhost:8065;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }

    location = /api/v4/posts/search {
        rewrite ^ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/posts/search break;
        proxy_pass http://localhost:8065;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }

    location ~ ^/api/v4/teams/([^/]+)/files/search$ {
        rewrite ^/api/v4/teams/([^/]+)/files/search$ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/$1/files/search break;
        proxy_pass http://localhost:8065;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }

    location = /api/v4/files/search {
        rewrite ^ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/files/search break;
        proxy_pass http://localhost:8065;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }

    location / {
        proxy_pass http://localhost:8065;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

If your Nginx config does not already define `$connection_upgrade`, add this in the `http` block:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}
```

## Apache

Enable the required modules:

```sh
a2enmod proxy proxy_http proxy_wstunnel rewrite headers
```

Virtual host:

```apache
<VirtualHost *:443>
    ServerName mattermost.example.com

    ProxyPreserveHost On
    ProxyRequests Off

    RewriteEngine On
    RewriteRule ^/api/v4/teams/([^/]+)/posts/search$ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/$1/posts/search [PT,L]
    RewriteRule ^/api/v4/posts/search$ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/posts/search [PT,L]
    RewriteRule ^/api/v4/teams/([^/]+)/files/search$ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/teams/$1/files/search [PT,L]
    RewriteRule ^/api/v4/files/search$ /plugins/com.mattermost.plugin-poor-mans-search/api/v1/files/search [PT,L]

    RequestHeader set X-Forwarded-Proto "https"
    RequestHeader set X-Forwarded-Ssl "on"

    ProxyPass /api/v4/websocket ws://localhost:8065/api/v4/websocket
    ProxyPassReverse /api/v4/websocket ws://localhost:8065/api/v4/websocket

    ProxyPass / http://localhost:8065/
    ProxyPassReverse / http://localhost:8065/
</VirtualHost>
```

## Site URL

Mattermost's `ServiceSettings.SiteURL` should match the public URL users browse through, for example:

```json
{
  "ServiceSettings": {
    "SiteURL": "https://mattermost.example.com"
  }
}
```

If `SiteURL` points at the upstream server while users browse through the proxy, websocket origin checks and generated links can fail.
