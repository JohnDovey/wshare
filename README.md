# wShare

wShare makes file sharing on a local network easy. Paths you share are tracked in a **SQLite** database, listed in a browser UI, and filtered by **`.gitignore`** and **`robots.txt`**.

## Tools

| Binary | Purpose |
|--------|---------|
| `wshare` | Share server — add a path (optional), serve download list |
| `wshare-admin` | Cross-platform console to browse / add / remove / cleanup the DB |

Both run on **Windows**, **Linux**, and **macOS**.

## Build

```bash
go build -o wshare ./cmd/wshare
go build -o wshare-admin ./cmd/wshare-admin
```

On Windows, outputs are `wshare.exe` and `wshare-admin.exe`.

## Share server

```bash
# Add a file or folder and start the server
./wshare -f ./photos
./wshare -f report.pdf

# Or only serve what is already in the database
./wshare

# Options
./wshare -f ./docs -p 8080 -db ./wshare.db
```

| Flag | Default | Description |
|------|---------|-------------|
| `-f` | _(none)_ | File or folder to add to the DB before serving |
| `-p` | `8080` | Preferred port; if busy, tries 8081, 8082, … |
| `-db` | `./wshare.db` | SQLite database path (`WSHARE_DB` env also works) |

When the server starts it prints the LAN and localhost URLs, e.g.:

```text
Share list:  http://192.168.1.10:8080/
Listening on port 8080
```

### Listing behaviour

- The home page lists **all files and folders** in the database, grouped by share root.
- **`.gitignore`** and **`robots.txt`** (`Disallow` for `User-agent: *`) under the shared root are respected when scanning; matching paths are not added.
- Paths that were shared but **no longer exist on disk** appear in **red**, without a download link. Clicking them shows a **“No Longer Available”** popup.
- **Files** download directly; each file’s download count is stored and shown in its row.
- **Folders** open for normal browsing (breadcrumbs + parent link). A **zip** checkbox sits next to each folder; only when it is checked does **Download** produce a zip archive. Zip downloads are counted per folder and shown in the row.

## Admin console

ServiceMonitor-style terminal UI (Bubble Tea + lipgloss): left **Shares** panel, right **Details**, bottom key help.

```bash
./wshare-admin
./wshare-admin -db ./wshare.db
```

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Select share |
| `a` | Add file or directory |
| `d` | Remove selected root (confirm `y`/`n`) |
| `c` | Cleanup missing-on-disk entries |
| `r` | Refresh |
| `?` | About (slides in from the left) |
| `q` | Quit |

The share server browser UI has the same **About** drawer (button in the title bar, or press `?`).

Use the same `-db` / `WSHARE_DB` as the server so both tools share one catalog.

The browser list uses the same layout language: title bar, left share list, right detail/entries panel, OK / MISS badges.

## Ignore rules

When adding a directory (or a file’s parent tree):

- **`.gitignore`** at the share root (plus always-ignored `.git/`, `wshare.db*`)
- **`robots.txt`** `Disallow:` lines for `User-agent: *` (or `wshare`)
- The files `.gitignore` and `robots.txt` themselves are not listed for download

## Legacy single-file mode

Older wShare used a one-shot map and fixed port 8080. The new flow always uses the SQLite catalog and the HTML list so multiple shares accumulate across runs.

## License

MIT
