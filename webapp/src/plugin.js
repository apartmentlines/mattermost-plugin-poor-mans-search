(function() {
    const pluginId = 'com.mattermost.plugin-poor-mans-search';
    const React = window.React;

    if (!React || !window.registerPlugin) {
        return;
    }

    const e = React.createElement;
    const defaultSearchResultDisplay = 'inline';
    const searchResultDisplaySettingName = 'PluginSettings.Plugins.com+mattermost+plugin-poor-mans-search.searchresultdisplay';
    let clientConfig = null;
    let clientConfigLoadedAt = 0;

    function apiPath(path) {
        return '/plugins/' + pluginId + '/api/v1' + path;
    }

    function getCookie(name) {
        const prefix = name + '=';
        return document.cookie.split(';').map((cookie) => cookie.trim()).find((cookie) => cookie.indexOf(prefix) === 0);
    }

    function getCSRFToken() {
        const cookie = getCookie('MMCSRF');
        if (!cookie) {
            return '';
        }
        return decodeURIComponent(cookie.replace('MMCSRF=', ''));
    }

    async function request(path, options) {
        const opts = Object.assign({
            credentials: 'same-origin',
            headers: {'Content-Type': 'application/json'},
        }, options || {});
        opts.headers = Object.assign({}, opts.headers);

        if (opts.method && opts.method.toUpperCase() !== 'GET') {
            const csrf = getCSRFToken();
            if (csrf) {
                opts.headers['X-CSRF-Token'] = csrf;
            }
            opts.headers['X-Requested-With'] = 'XMLHttpRequest';
        }

        const response = await fetch(apiPath(path), opts);

        if (!response.ok) {
            const text = await response.text();
            throw new Error(text || response.statusText);
        }

        const text = await response.text();
        return text ? JSON.parse(text) : {};
    }

    async function getClientConfig() {
        const now = Date.now();
        if (clientConfig && now - clientConfigLoadedAt < 5000) {
            return clientConfig;
        }

        clientConfig = await request('/config/client');
        clientConfigLoadedAt = now;
        return clientConfig;
    }

    function rewriteSearchToFind(message) {
        const leadingWhitespace = message.match(/^\s*/)[0];
        const commandText = message.slice(leadingWhitespace.length);
        if (!/^\/search(?=$|\s)/i.test(commandText)) {
            return message;
        }
        return leadingWhitespace + commandText.replace(/^\/search(?=$|\s)/i, '/find');
    }

    async function handleSlashCommandWillBePosted(message, args) {
        if (rewriteSearchToFind(message) === message) {
            return {message, args};
        }

        try {
            const config = await getClientConfig();
            if ((config.search_result_display || defaultSearchResultDisplay) !== 'inline') {
                return {message, args};
            }
        } catch (err) {
            return {message, args};
        }

        return {message: rewriteSearchToFind(message), args};
    }

    function selectedSearchResultDisplaySetting() {
        const selected = document.querySelector('input[name="' + searchResultDisplaySettingName + '"]:checked');
        return selected ? selected.value : '';
    }

    function isSearchResultDisplaySettingInput(input) {
        return input && input.name === searchResultDisplaySettingName;
    }

    function formatDate(value) {
        if (!value) {
            return '';
        }
        return new Date(value).toLocaleString();
    }

    function formatRuntime(value) {
        if (!value) {
            return '';
        }
        const seconds = Math.round(value / 1000);
        if (seconds < 60) {
            return seconds + ' seconds';
        }
        const minutes = Math.floor(seconds / 60);
        const remaining = seconds % 60;
        return minutes + 'm ' + remaining + 's';
    }

    function statusColor(status) {
        if (status === 'success') {
            return '#3db887';
        }
        if (status === 'running') {
            return '#1c58d9';
        }
        return '#d24b4e';
    }

    function buildRows(status) {
        const rows = [];
        if (status && status.rebuild && status.rebuild.running) {
            const rebuild = status.rebuild;
            const processed = (rebuild.posts_indexed || 0) + (rebuild.files_indexed || 0);
            rows.push({
                id: rebuild.id || 'running',
                status: 'running',
                completed_at: 0,
                run_time_millis: Date.now() - rebuild.started_at,
                details: processed > 0 ?
                    rebuild.posts_indexed + ' posts processed, ' + rebuild.files_indexed + ' files processed' :
                    'Rebuild in progress',
            });
        }
        if (status && Array.isArray(status.history)) {
            status.history.forEach((row) => rows.push(row));
        }
        return rows;
    }

    function statusSummary(loading, engine, rebuild) {
        if (loading) {
            return 'Loading index status...';
        }
        if (rebuild && rebuild.running) {
            return 'Index rebuild running. ' + (rebuild.posts_indexed || 0) + ' messages and ' + (rebuild.files_indexed || 0) + ' files processed so far.';
        }
        if (engine.active) {
            return 'Index active. ' + (engine.post_docs || 0) + ' message documents and ' + (engine.file_docs || 0) + ' file documents are indexed.';
        }
        return 'Index inactive. Save a valid index directory or re-enable the plugin before rebuilding.';
    }

    function SearchAdminSetting(props) {
        const [status, setStatus] = React.useState(null);
        const [config, setConfig] = React.useState(null);
        const [searchResultDisplay, setSearchResultDisplay] = React.useState('');
        const [loading, setLoading] = React.useState(true);
        const [working, setWorking] = React.useState('');
        const [error, setError] = React.useState('');
        const [notice, setNotice] = React.useState('');

        const loadStatus = React.useCallback(async function() {
            try {
                const data = await request('/index/status');
                setStatus(data);
                setError('');
            } catch (err) {
                setError(err.message || String(err));
            } finally {
                setLoading(false);
            }
        }, []);

        React.useEffect(function() {
            loadStatus();
            getClientConfig().then(function(data) {
                setConfig(data);
                setSearchResultDisplay(selectedSearchResultDisplaySetting() || data.search_result_display || defaultSearchResultDisplay);
            }).catch(function() {
                setSearchResultDisplay(selectedSearchResultDisplaySetting() || defaultSearchResultDisplay);
            });
            const interval = window.setInterval(loadStatus, 3000);
            return function() {
                window.clearInterval(interval);
            };
        }, [loadStatus]);

        React.useEffect(function() {
            function handleSearchResultDisplayChange(event) {
                if (isSearchResultDisplaySettingInput(event.target)) {
                    setSearchResultDisplay(event.target.value || defaultSearchResultDisplay);
                }
            }

            setSearchResultDisplay(selectedSearchResultDisplaySetting() || (config && config.search_result_display) || defaultSearchResultDisplay);
            document.addEventListener('change', handleSearchResultDisplayChange, true);
            return function() {
                document.removeEventListener('change', handleSearchResultDisplayChange, true);
            };
        }, [config]);

        async function startRebuild() {
            setWorking('rebuild');
            try {
                await request('/index/rebuild', {method: 'POST'});
                setNotice('Index rebuild started.');
                await loadStatus();
            } catch (err) {
                setError(err.message || String(err));
            } finally {
                setWorking('');
            }
        }

        async function purgeIndexes() {
            if (!window.confirm('Purge the plugin Bleve indexes? Search results will be incomplete until indexing is rebuilt.')) {
                return;
            }

            setWorking('purge');
            try {
                await request('/index/purge', {method: 'POST'});
                setNotice('Index purged. Rebuild history was cleared.');
                await loadStatus();
            } catch (err) {
                setError(err.message || String(err));
            } finally {
                setWorking('');
            }
        }

        const engine = status && status.engine ? status.engine : {};
        const rebuild = status && status.rebuild ? status.rebuild : {};
        const rows = buildRows(status);
        const disabled = props.disabled || working !== '' || !engine.active;
        const inlineMode = (searchResultDisplay || defaultSearchResultDisplay) === 'inline';
        const sidebarMode = (searchResultDisplay || defaultSearchResultDisplay) === 'sidebar';

        return e('div', {className: 'poor-mans-search-admin'},
            inlineMode ? e('div', {className: 'alert alert-warning', style: {marginBottom: '12px'}},
                'Inline mode rewrites /search slash commands to inline plugin results. Mattermost\'s search box still uses native search unless sidebar proxying is configured.'
            ) : null,
            sidebarMode ? e('div', {className: 'alert alert-warning', style: {marginBottom: '12px'}},
                'Sidebar mode requires proxying Mattermost search API requests to this plugin\'s search API endpoints; see setup instructions for details.'
            ) : null,
            e('div', {style: {marginBottom: '12px', color: 'rgba(63,67,80,0.88)'}},
                statusSummary(loading, engine, rebuild)
            ),
            error ? e('div', {className: 'alert alert-danger', style: {marginBottom: '12px'}}, error) : null,
            notice && !error ? e('div', {className: 'alert alert-success', style: {marginBottom: '12px'}}, notice) : null,
            e('div', {style: {display: 'flex', gap: '8px', marginBottom: '12px'}},
                e('button', {
                    type: 'button',
                    className: 'btn btn-primary',
                    disabled,
                    onClick: startRebuild,
                }, working === 'rebuild' || rebuild.running ? 'Indexing...' : 'Index Now'),
                e('button', {
                    type: 'button',
                    className: 'btn btn-danger',
                    disabled,
                    onClick: purgeIndexes,
                }, working === 'purge' ? 'Purging...' : 'Purge Index')
            ),
            e('div', {style: {marginBottom: '16px', color: 'rgba(63,67,80,0.72)'}},
                'All available posts and files will be indexed from oldest to newest. Search is available during indexing, but results may be incomplete until the job finishes.'
            ),
            e('table', {className: 'table', style: {maxWidth: '760px', background: '#fff'}},
                e('thead', null,
                    e('tr', null,
                        e('th', null, 'Status'),
                        e('th', null, 'Finish Time'),
                        e('th', null, 'Run Time'),
                        e('th', null, 'Details')
                    )
                ),
                e('tbody', null,
                    rows.length === 0 ?
                        e('tr', null, e('td', {colSpan: 4}, 'No rebuilds have completed yet.')) :
                        rows.map((row) => e('tr', {key: row.id || row.started_at},
                            e('td', {style: {color: statusColor(row.status)}}, row.status === 'running' ? 'Running' : row.status === 'success' ? 'Success' : 'Error'),
                            e('td', null, formatDate(row.completed_at)),
                            e('td', null, formatRuntime(row.run_time_millis)),
                            e('td', null, row.details || '')
                        ))
                )
            ),
            e('div', {style: {fontSize: '12px', color: 'rgba(63,67,80,0.72)'}},
                engine.index_dir ? 'Index directory: ' + engine.index_dir : ''
            )
        );
    }

    window.registerPlugin(pluginId, {
        initialize: function(registry) {
            if (registry.registerSlashCommandWillBePostedHook) {
                registry.registerSlashCommandWillBePostedHook(handleSlashCommandWillBePosted);
            }
            registry.registerAdminConsoleCustomSetting('SearchAdmin', SearchAdminSetting, {showTitle: true});
        },
    });
}());
