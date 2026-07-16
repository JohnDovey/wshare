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

	entries, err := s.Store.ListEntries(0)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	store.MarkAvailability(entries)

	roots, err := s.Store.ListRoots()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>wShare — Shared files</title>
<link rel="stylesheet" href="/static/style.css">
<script>
function unavailable(name) {
  alert("No Longer Available");
  return false;
}
</script>
</head>
<body>
<header>
  <h1>wShare</h1>
  <p class="sub">Files and folders available for download on this network</p>
</header>
<main>
`)

	if len(entries) == 0 {
		b.WriteString(`<p class="empty">No files are shared yet. Use the share server or admin tool to add paths.</p>`)
	} else {
		// Group by root
		byRoot := map[int64][]store.Entry{}
		for _, e := range entries {
			byRoot[e.RootID] = append(byRoot[e.RootID], e)
		}

		for _, r := range roots {
			list := byRoot[r.ID]
			if len(list) == 0 {
				continue
			}
			b.WriteString("<section class=\"root\">\n")
			b.WriteString("<h2>" + html.EscapeString(r.Name) + "</h2>\n")
			b.WriteString("<p class=\"path\">" + html.EscapeString(r.Path) + "</p>\n")
			b.WriteString("<table><thead><tr><th>Name</th><th>Type</th><th>Size</th><th></th></tr></thead><tbody>\n")
			for _, e := range list {
				// Skip the root dir entry as a self-download row when there are children — still show files/subdirs.
				typeLabel := "file"
				if e.IsDir {
					typeLabel = "folder"
				}
				sizeStr := formatSize(e.Size)
				if e.IsDir {
					sizeStr = "—"
				}
				// Relative display path under root
				rel := e.Name
				if relPath, err := filepath.Rel(r.Path, e.Path); err == nil && relPath != "." {
					rel = relPath
				} else if e.Path == r.Path {
					rel = e.Name + " (root)"
				}

				if e.Available {
					href := fmt.Sprintf("/download/%d", e.ID)
					b.WriteString(fmt.Sprintf(
						`<tr class="ok"><td><a href="%s">%s</a></td><td>%s</td><td>%s</td><td><a class="btn" href="%s">Download</a></td></tr>`,
						href, html.EscapeString(filepath.ToSlash(rel)), typeLabel, sizeStr, href,
					))
				} else {
					b.WriteString(fmt.Sprintf(
						`<tr class="missing"><td><span class="gone" onclick="return unavailable(%q)">%s</span></td><td>%s</td><td>%s</td><td><button type="button" class="btn gone" onclick="return unavailable(%q)">No Longer Available</button></td></tr>`,
						e.Name, html.EscapeString(filepath.ToSlash(rel)), typeLabel, sizeStr, e.Name,
					))
				}
				b.WriteByte('\n')
			}
			b.WriteString("</tbody></table>\n</section>\n")
		}
	}

	b.WriteString(`</main>
<footer><p>wShare — local network file sharing</p></footer>
</body>
</html>`)
	_, _ = io.WriteString(w, b.String())
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
<p style="color:red">No Longer Available: %s</p><p><a href="/">Back to list</a></p></body></html>`,
			html.EscapeString(entry.Name))
		return
	}

	if info.IsDir() {
		zipName := entry.Name + ".zip"
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
		if err := zipDir(entry.Path, w); err != nil {
			log.Println("zip error:", err)
		}
		return
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

// GetOutboundIP returns the preferred outbound IP of this machine.
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}

const css = `
:root {
  --bg: #0f1419;
  --card: #1a2332;
  --text: #e7ecf3;
  --muted: #8b9bb4;
  --accent: #3d8bfd;
  --missing: #e35d6a;
  --ok-border: #2a3a4f;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
  background: var(--bg);
  color: var(--text);
  line-height: 1.5;
}
header, footer {
  text-align: center;
  padding: 1.5rem 1rem;
}
header h1 { margin: 0; font-weight: 700; letter-spacing: -0.02em; }
.sub { color: var(--muted); margin: 0.4rem 0 0; }
main { max-width: 960px; margin: 0 auto; padding: 0 1rem 2rem; }
.root {
  background: var(--card);
  border-radius: 12px;
  padding: 1rem 1.25rem 1.25rem;
  margin-bottom: 1.25rem;
  border: 1px solid var(--ok-border);
}
.root h2 { margin: 0 0 0.25rem; font-size: 1.15rem; }
.path { color: var(--muted); font-size: 0.85rem; margin: 0 0 0.75rem; word-break: break-all; }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 0.55rem 0.4rem; border-bottom: 1px solid var(--ok-border); }
th { color: var(--muted); font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.04em; }
a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }
.btn {
  display: inline-block;
  padding: 0.25rem 0.65rem;
  border-radius: 6px;
  background: #243247;
  border: 1px solid var(--ok-border);
  color: var(--text);
  font-size: 0.85rem;
  cursor: pointer;
}
a.btn:hover { background: #2d3f5a; text-decoration: none; }
tr.missing td, .gone { color: var(--missing) !important; }
span.gone { cursor: pointer; text-decoration: underline dotted; }
button.gone {
  background: transparent;
  border-color: var(--missing);
  color: var(--missing);
}
.empty { text-align: center; color: var(--muted); padding: 2rem; }
footer { color: var(--muted); font-size: 0.85rem; }
`
