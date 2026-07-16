package server

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JohnDovey/wshare/internal/about"
	"github.com/JohnDovey/wshare/internal/store"
)

// Server serves the share listing and file downloads.
type Server struct {
	Store *store.Store
	IP    string
	Port  int
}

// Handler returns the HTTP mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/download/", s.handleDownload)
	mux.HandleFunc("/static/style.css", s.handleCSS)
	return mux
}

func (s *Server) handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(css))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	allEntries, err := s.Store.ListEntries(0)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	store.MarkAvailability(allEntries)

	roots, err := s.Store.ListRoots()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	okCount := map[int64]int{}
	missCount := map[int64]int{}
	for _, e := range allEntries {
		if e.Available {
			okCount[e.RootID]++
		} else {
			missCount[e.RootID]++
		}
	}

	var selectedRoot int64
	if q := r.URL.Query().Get("root"); q != "" {
		if id, err := strconv.ParseInt(q, 10, 64); err == nil {
			selectedRoot = id
		}
	}
	if selectedRoot == 0 && len(roots) > 0 {
		selectedRoot = roots[0].ID
	}

	var dirID int64
	if q := r.URL.Query().Get("dir"); q != "" {
		if id, err := strconv.ParseInt(q, 10, 64); err == nil {
			dirID = id
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>wShare</title>
<link rel="stylesheet" href="/static/style.css">
<script>
function unavailable(name) {
  alert("No Longer Available");
  return false;
}
// Folder download only when its "zip" checkbox is checked.
function folderDownload(id, name) {
  var cb = document.getElementById("zip-" + id);
  if (!cb || !cb.checked) {
    alert("Check \"zip\" next to the folder to download it as a zip archive.\nOtherwise click the folder name to open it.");
    return false;
  }
  window.location.href = "/download/" + id + "?zip=1";
  return false;
}
function openAbout() {
  document.getElementById("about-drawer").classList.add("open");
  document.getElementById("about-backdrop").classList.add("open");
  document.body.classList.add("about-open");
}
function closeAbout() {
  document.getElementById("about-drawer").classList.remove("open");
  document.getElementById("about-backdrop").classList.remove("open");
  document.body.classList.remove("about-open");
}
document.addEventListener("keydown", function(e) {
  if (e.key === "Escape") closeAbout();
  if (e.key === "?" && !e.ctrlKey && !e.metaKey && !e.altKey) {
    var t = e.target && e.target.tagName;
    if (t === "INPUT" || t === "TEXTAREA") return;
    e.preventDefault();
    var d = document.getElementById("about-drawer");
    if (d.classList.contains("open")) closeAbout(); else openAbout();
  }
});
</script>
</head>
<body>
<div id="about-backdrop" class="about-backdrop" onclick="closeAbout()" aria-hidden="true"></div>
<aside id="about-drawer" class="about-drawer" aria-label="About wShare" role="dialog">
  <div class="about-drawer-head">
    <span class="title-badge">About</span>
    <button type="button" class="about-close" onclick="closeAbout()" title="Close">×</button>
  </div>
  <div class="about-drawer-body">
`)
	b.WriteString(about.HTMLBody(about.AppServer))
	b.WriteString(`
  </div>
</aside>
<header class="titlebar">
  <button type="button" class="about-open-btn" onclick="openAbout()" title="About (press ?)">☰ About</button>
  <span class="title-badge">wShare</span>
  <span class="title-dim">local network file sharing</span>
</header>
<div class="layout">
<aside class="panel left">
  <div class="panel-title">Shares</div>
`)

	if len(roots) == 0 {
		b.WriteString(`<p class="empty">No shares yet. Use <code>wshare -f …</code> or <code>wshare-admin</code>.</p>`)
	} else {
		b.WriteString(`<ul class="share-list">`)
		for _, root := range roots {
			cls := "share-item"
			if root.ID == selectedRoot {
				cls += " selected"
			}
			ok, miss := okCount[root.ID], missCount[root.ID]
			badge := "OK"
			badgeCls := "badge ok"
			if miss > 0 && ok == 0 {
				badge = "MISS"
				badgeCls = "badge miss"
			} else if miss > 0 {
				badge = "MIX"
				badgeCls = "badge miss"
			} else if ok == 0 {
				badge = "—"
				badgeCls = "badge dim"
			}
			kind := "file"
			if root.IsDir {
				kind = "folder"
			}
			summary := fmt.Sprintf("%d items", ok+miss)
			if miss > 0 {
				summary = fmt.Sprintf("%d ok · %d missing", ok, miss)
			}
			fmt.Fprintf(&b,
				`<li><a class="%s" href="/?root=%d"><span class="%s">%s</span> %s</a><div class="share-meta">%s · %s</div></li>`,
				cls, root.ID, badgeCls, badge, html.EscapeString(root.Name),
				kind, summary,
			)
		}
		b.WriteString(`</ul>`)
	}

	b.WriteString(`</aside>
<main class="panel right">
  <div class="panel-title">Details</div>
`)

	if len(roots) == 0 || selectedRoot == 0 {
		b.WriteString(`<p class="empty">Select a share from the left, or add paths with the admin tool.</p>`)
	} else {
		var sel *store.Root
		for i := range roots {
			if roots[i].ID == selectedRoot {
				sel = &roots[i]
				break
			}
		}
		if sel == nil {
			b.WriteString(`<p class="empty">Share not found.</p>`)
		} else {
			s.renderBrowse(&b, sel, dirID, okCount[sel.ID], missCount[sel.ID])
		}
	}

	b.WriteString(`</main>
</div>
<footer class="helpbar">
  <span class="key">About</span> or <span class="key">?</span> attribution &nbsp;
  <span class="key">folder name</span> open folder &nbsp;
  <span class="key">zip ☑</span> then Download = zip archive &nbsp;
  <span class="key">file</span> download directly &nbsp;
  missing items show in amber
</footer>
</body>
</html>`)
	_, _ = io.WriteString(w, b.String())
}

func (s *Server) renderBrowse(b *strings.Builder, sel *store.Root, dirID int64, okN, missN int) {
	// Resolve which folder we are browsing.
	currentDirID, list, crumbs, atShareRoot, err := s.resolveListing(sel, dirID)
	if err != nil {
		fmt.Fprintf(b, `<p class="empty">%s</p>`, html.EscapeString(err.Error()))
		return
	}
	store.MarkAvailability(list)

	kind := "file"
	if sel.IsDir {
		kind = "folder"
	}
	status := "ALL AVAILABLE"
	statusCls := "status ok"
	if missN > 0 && okN == 0 {
		status = "ALL MISSING"
		statusCls = "status miss"
	} else if missN > 0 {
		status = fmt.Sprintf("PARTIAL (%d missing)", missN)
		statusCls = "status miss"
	}

	fmt.Fprintf(b, `<h1 class="detail-name">%s</h1>
<div class="kv"><span class="k">type</span> %s</div>
<div class="kv"><span class="k">path</span> <span class="path">%s</span></div>
<div class="kv"><span class="k">status</span> <span class="%s">%s</span></div>
`,
		html.EscapeString(sel.Name),
		kind,
		html.EscapeString(sel.Path),
		statusCls, status,
	)

	// Breadcrumb trail for folder navigation.
	b.WriteString(`<nav class="crumbs">`)
	fmt.Fprintf(b, `<a href="/?root=%d">%s</a>`, sel.ID, html.EscapeString(sel.Name))
	for _, c := range crumbs {
		fmt.Fprintf(b, ` <span class="crumb-sep">/</span> <a href="/?root=%d&amp;dir=%d">%s</a>`,
			sel.ID, c.ID, html.EscapeString(c.Name))
	}
	b.WriteString(`</nav>`)

	// Parent link only when inside a subfolder (not at share root).
	if !atShareRoot && currentDirID > 0 {
		parentLink := fmt.Sprintf("/?root=%d", sel.ID)
		cur, _ := s.Store.GetEntry(currentDirID)
		if cur != nil && cur.ParentID.Valid {
			parentEnt, err := s.Store.GetEntry(cur.ParentID.Int64)
			if err == nil && parentEnt.Path != sel.Path {
				parentLink = fmt.Sprintf("/?root=%d&dir=%d", sel.ID, cur.ParentID.Int64)
			}
		}
		fmt.Fprintf(b, `<p class="up-link"><a href="%s">↑ parent folder</a></p>`, parentLink)
	}

	b.WriteString(`<h2 class="section-title">Contents</h2>
<table>
<thead><tr><th>Status</th><th>Name</th><th>Type</th><th>Size</th><th title="File downloads, or zip downloads for folders">Downloads</th><th>Zip</th><th></th></tr></thead>
<tbody>
`)

	if len(list) == 0 {
		b.WriteString(`<tr><td colspan="7" class="empty">This folder is empty (or only ignored items).</td></tr>`)
	}

	for _, e := range list {
		typeLabel := "file"
		if e.IsDir {
			typeLabel = "folder"
		}
		sizeStr := formatSize(e.Size)
		if e.IsDir {
			sizeStr = "—"
		}
		name := e.Name
		dlLabel := formatDownloads(e.DownloadCount, e.IsDir)

		if !e.Available {
			fmt.Fprintf(b,
				`<tr class="missing"><td><span class="badge miss">MISS</span></td><td><span class="gone" onclick="return unavailable(%q)">%s</span></td><td>%s</td><td>%s</td><td class="dl-count">%s</td><td></td><td><button type="button" class="btn gone" onclick="return unavailable(%q)">No Longer Available</button></td></tr>`,
				e.Name, html.EscapeString(name), typeLabel, sizeStr, html.EscapeString(dlLabel), e.Name,
			)
			continue
		}

		if e.IsDir {
			// Navigate by default; zip only via checkbox + Download.
			openHref := fmt.Sprintf("/?root=%d&dir=%d", sel.ID, e.ID)
			fmt.Fprintf(b,
				`<tr class="ok">
<td><span class="badge ok">OK</span></td>
<td><a class="folder-link" href="%s">📁 %s</a></td>
<td>%s</td>
<td>%s</td>
<td class="dl-count" title="Times this folder was downloaded as a zip">%s</td>
<td class="zip-cell"><label class="zip-label" title="Check to download this folder as a zip"><input type="checkbox" id="zip-%d" class="zip-cb"> zip</label></td>
<td><button type="button" class="btn" onclick="return folderDownload(%d, %q)">Download</button></td>
</tr>`,
				openHref, html.EscapeString(name), typeLabel, sizeStr, html.EscapeString(dlLabel), e.ID, e.ID, e.Name,
			)
		} else {
			href := fmt.Sprintf("/download/%d", e.ID)
			fmt.Fprintf(b,
				`<tr class="ok">
<td><span class="badge ok">OK</span></td>
<td><a href="%s">%s</a></td>
<td>%s</td>
<td>%s</td>
<td class="dl-count" title="Times this file was downloaded">%s</td>
<td></td>
<td><a class="btn" href="%s">Download</a></td>
</tr>`,
				href, html.EscapeString(name), typeLabel, sizeStr, html.EscapeString(dlLabel), href,
			)
		}
		b.WriteByte('\n')
	}
	b.WriteString("</tbody></table>\n")
}

// resolveListing returns the folder being browsed, its direct children, breadcrumbs,
// and whether the view is the top of the share.
func (s *Server) resolveListing(sel *store.Root, dirID int64) (currentDirID int64, list []store.Entry, crumbs []store.Entry, atShareRoot bool, err error) {
	// File share: single top-level file entry.
	if !sel.IsDir {
		list, err = s.Store.ListChildren(sel.ID, nil)
		return 0, list, nil, true, err
	}

	// Directory share: start at the root entry for this share.
	rootEntryID, err := s.findRootEntryID(sel)
	if err != nil {
		return 0, nil, nil, false, err
	}

	if dirID == 0 || dirID == rootEntryID {
		// Top of share: show children of the shared folder (not the folder row itself).
		pid := rootEntryID
		list, err = s.Store.ListChildren(sel.ID, &pid)
		return rootEntryID, list, nil, true, err
	}

	// Validate dir belongs to this root and is a directory.
	ent, err := s.Store.GetEntry(dirID)
	if err != nil || ent.RootID != sel.ID {
		return 0, nil, nil, false, fmt.Errorf("folder not found in this share")
	}
	if !ent.IsDir {
		// If someone passes a file id, show sibling listing of its parent.
		if ent.ParentID.Valid {
			dirID = ent.ParentID.Int64
			ent, err = s.Store.GetEntry(dirID)
			if err != nil {
				return 0, nil, nil, false, err
			}
		} else {
			dirID = rootEntryID
		}
		if dirID == rootEntryID {
			pid := rootEntryID
			list, err = s.Store.ListChildren(sel.ID, &pid)
			return rootEntryID, list, nil, true, err
		}
	}

	// Breadcrumbs from root entry down to current (exclude share root entry name duplicate).
	crumbs = s.breadcrumb(sel, dirID, rootEntryID)

	pid := dirID
	list, err = s.Store.ListChildren(sel.ID, &pid)
	return dirID, list, crumbs, false, err
}

func (s *Server) findRootEntryID(sel *store.Root) (int64, error) {
	top, err := s.Store.ListChildren(sel.ID, nil)
	if err != nil {
		return 0, err
	}
	for _, e := range top {
		if e.Path == sel.Path {
			return e.ID, nil
		}
	}
	// Fallback: first top-level dir, or first entry.
	for _, e := range top {
		if e.IsDir {
			return e.ID, nil
		}
	}
	if len(top) > 0 {
		return top[0].ID, nil
	}
	return 0, fmt.Errorf("share has no catalog entries")
}

func (s *Server) breadcrumb(sel *store.Root, dirID, rootEntryID int64) []store.Entry {
	var chain []store.Entry
	id := dirID
	seen := map[int64]bool{}
	for id > 0 && !seen[id] {
		seen[id] = true
		ent, err := s.Store.GetEntry(id)
		if err != nil || ent.RootID != sel.ID {
			break
		}
		// Don't include the share root entry itself in crumbs (shown as share name).
		if ent.ID != rootEntryID {
			chain = append(chain, *ent)
		}
		if !ent.ParentID.Valid {
			break
		}
		id = ent.ParentID.Int64
	}
	// Reverse so root→current
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/download/")
	idStr = strings.Trim(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	entry, err := s.Store.GetEntry(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	info, err := os.Stat(entry.Path)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><body><script>alert("No Longer Available");history.back();</script>
<p style="color:#ffaf00">No Longer Available: %s</p><p><a href="/">Back to list</a></p></body></html>`,
			html.EscapeString(entry.Name))
		return
	}

	if info.IsDir() {
		// Folders are only zipped when explicitly requested (?zip=1).
		if r.URL.Query().Get("zip") != "1" {
			// Redirect into folder browser instead of zipping.
			http.Redirect(w, r, fmt.Sprintf("/?root=%d&dir=%d", entry.RootID, entry.ID), http.StatusFound)
			return
		}
		if n, err := s.Store.IncrementDownloadCount(entry.ID); err != nil {
			log.Println("download count:", err)
		} else {
			log.Printf("zip download %q → %d total", entry.Name, n)
		}
		zipName := entry.Name + ".zip"
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
		if err := zipDir(entry.Path, w); err != nil {
			log.Println("zip error:", err)
		}
		return
	}

	if n, err := s.Store.IncrementDownloadCount(entry.ID); err != nil {
		log.Println("download count:", err)
	} else {
		log.Printf("file download %q → %d total", entry.Name, n)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, entry.Name))
	http.ServeFile(w, r, entry.Path)
}

func zipDir(source string, w io.Writer) error {
	archive := zip.NewWriter(w)
	defer archive.Close()

	baseDir := filepath.Base(source)
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(baseDir, rel))
		if info.IsDir() {
			header.Name += "/"
			_, err = archive.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, err = io.Copy(writer, f)
		return err
	})
}

func formatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}

// formatDownloads shows how many times a file was downloaded,
// or a folder was downloaded as a zip.
func formatDownloads(n int64, isDir bool) string {
	if isDir {
		if n == 1 {
			return "1 zip"
		}
		return fmt.Sprintf("%d zips", n)
	}
	if n == 1 {
		return "1×"
	}
	return fmt.Sprintf("%d×", n)
}

// GetOutboundIP returns the preferred outbound IP of this machine.
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}

// CSS mirrors ServiceMonitor lipgloss colors (ANSI 24/62/78/117/214/230/240…).
const css = `
:root {
  --bg: #0c0c0c;
  --panel: #121212;
  --border: #585858;
  --text: #e4e4e4;
  --title-fg: #ffffd7;
  --title-bg: #005f87;
  --panel-title: #87d7ff;
  --selected-bg: #5f5faf;
  --selected-fg: #ffffd7;
  --ok: #5fd787;
  --miss: #ffaf00;
  --err: #ff5f5f;
  --dim: #767676;
  --help: #8a8a8a;
  --key: #87d7ff;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  min-height: 100vh;
  font-family: ui-monospace, "Cascadia Code", "SF Mono", Consolas, monospace;
  background: var(--bg);
  color: var(--text);
  line-height: 1.45;
  display: flex;
  flex-direction: column;
}
.titlebar {
  display: flex;
  align-items: baseline;
  gap: 0.75rem;
  padding: 0.55rem 0.75rem;
  flex-wrap: wrap;
}
.title-badge {
  background: var(--title-bg);
  color: var(--title-fg);
  font-weight: 700;
  padding: 0.15rem 0.55rem;
}
.title-dim { color: var(--dim); font-size: 0.9rem; }
.about-open-btn {
  font: inherit;
  font-size: 0.85rem;
  font-weight: 700;
  color: var(--title-fg);
  background: var(--title-bg);
  border: 1px solid #0077a8;
  border-radius: 4px;
  padding: 0.15rem 0.55rem;
  cursor: pointer;
}
.about-open-btn:hover { filter: brightness(1.15); }
.about-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.28s ease;
  z-index: 90;
}
.about-backdrop.open {
  opacity: 1;
  pointer-events: auto;
}
.about-drawer {
  position: fixed;
  top: 0;
  left: 0;
  height: 100%;
  width: min(380px, 92vw);
  background: var(--panel);
  border-right: 1px solid var(--border);
  box-shadow: 8px 0 24px rgba(0, 0, 0, 0.45);
  transform: translateX(-105%);
  transition: transform 0.28s ease;
  z-index: 100;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.about-drawer.open { transform: translateX(0); }
.about-drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.65rem 0.75rem;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.about-close {
  font: inherit;
  font-size: 1.35rem;
  line-height: 1;
  color: var(--text);
  background: transparent;
  border: 1px solid var(--border);
  border-radius: 4px;
  width: 2rem;
  height: 2rem;
  cursor: pointer;
}
.about-close:hover { background: #1e1e1e; }
.about-drawer-body {
  padding: 0.75rem 1rem 1.25rem;
  overflow: auto;
  flex: 1;
  font-size: 0.88rem;
  overflow-wrap: anywhere;
  word-wrap: break-word;
  word-break: break-word;
  hyphens: auto;
}
.about-drawer-body h2 {
  margin: 0 0 0.75rem;
  font-size: 1.15rem;
  color: #fff;
}
.about-drawer-body h3 {
  margin: 1rem 0 0.35rem;
  font-size: 0.9rem;
  color: var(--panel-title);
}
.about-drawer-body p {
  margin: 0.35rem 0;
  color: var(--text);
  line-height: 1.55;
  max-width: 100%;
}
.about-drawer-body a { color: var(--key); }
.about-url {
  display: block;
  margin-top: 0.2rem;
  font-size: 0.8rem;
  color: var(--key);
  overflow-wrap: anywhere;
  word-break: break-all;
  line-height: 1.4;
}
.about-contrib { list-style: none; margin: 0.35rem 0 0; padding: 0; }
.about-contrib li {
  margin: 0 0 0.75rem;
  padding-bottom: 0.65rem;
  border-bottom: 1px solid #2a2a2a;
}
.about-note {
  color: var(--dim);
  font-size: 0.82rem;
  margin-top: 0.25rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.about-license { color: var(--help); margin-top: 1rem !important; }
.layout {
  flex: 1;
  display: grid;
  grid-template-columns: minmax(220px, 280px) 1fr;
  gap: 0.5rem;
  padding: 0 0.5rem 0.5rem;
  min-height: 0;
}
@media (max-width: 720px) {
  .layout { grid-template-columns: 1fr; }
}
.panel {
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.65rem 0.75rem 0.85rem;
  min-height: 12rem;
  overflow: auto;
}
.panel-title {
  color: var(--panel-title);
  font-weight: 700;
  margin-bottom: 0.65rem;
  font-size: 0.95rem;
}
.share-list { list-style: none; margin: 0; padding: 0; }
.share-list li { margin-bottom: 0.35rem; }
.share-item {
  display: block;
  color: var(--text);
  text-decoration: none;
  padding: 0.2rem 0.35rem;
  border-radius: 3px;
}
.share-item.selected {
  background: var(--selected-bg);
  color: var(--selected-fg);
  font-weight: 700;
}
.share-item:hover:not(.selected) { background: #1e1e1e; }
.share-meta {
  color: var(--dim);
  font-size: 0.8rem;
  padding: 0 0.35rem 0.25rem 0.55rem;
}
.badge {
  display: inline-block;
  min-width: 2.4rem;
  text-align: center;
  font-weight: 700;
  font-size: 0.75rem;
}
.badge.ok { color: var(--ok); }
.badge.miss { color: var(--miss); }
.badge.dim { color: var(--dim); }
.detail-name {
  margin: 0 0 0.5rem;
  font-size: 1.15rem;
  color: #ffffff;
}
.kv { margin: 0.15rem 0; font-size: 0.9rem; }
.k { color: var(--dim); display: inline-block; min-width: 4.5rem; }
.path { word-break: break-all; color: var(--help); }
.status.ok { color: var(--ok); font-weight: 700; }
.status.miss { color: var(--miss); font-weight: 700; }
.section-title {
  color: var(--panel-title);
  font-size: 0.95rem;
  margin: 1rem 0 0.5rem;
}
.crumbs {
  margin: 0.6rem 0 0.25rem;
  font-size: 0.9rem;
  word-break: break-all;
}
.crumb-sep { color: var(--dim); }
.up-link { margin: 0.35rem 0 0.5rem; font-size: 0.9rem; }
.folder-link { font-weight: 600; }
.dl-count {
  color: var(--dim);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  font-size: 0.85rem;
}
.zip-cell { white-space: nowrap; }
.zip-label {
  color: var(--dim);
  font-size: 0.8rem;
  cursor: pointer;
  user-select: none;
}
.zip-cb { vertical-align: middle; margin-right: 0.25rem; accent-color: var(--key); }
table { width: 100%; border-collapse: collapse; font-size: 0.9rem; }
th, td { text-align: left; padding: 0.4rem 0.35rem; border-bottom: 1px solid #2a2a2a; }
th { color: var(--dim); font-weight: 600; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; }
a { color: var(--key); text-decoration: none; }
a:hover { text-decoration: underline; }
.btn {
  display: inline-block;
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  background: #1a1a1a;
  border: 1px solid var(--border);
  color: var(--text);
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}
a.btn:hover, button.btn:hover { background: #2a2a2a; text-decoration: none; }
tr.missing td, .gone { color: var(--miss) !important; }
span.gone { cursor: pointer; text-decoration: underline dotted; }
button.gone {
  background: transparent;
  border-color: var(--miss);
  color: var(--miss);
}
.empty { color: var(--dim); padding: 1rem 0.25rem; }
code { color: var(--panel-title); }
.helpbar {
  padding: 0.45rem 0.75rem 0.65rem;
  color: var(--help);
  font-size: 0.8rem;
}
.key { color: var(--key); font-weight: 700; }
`
