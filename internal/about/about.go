// Package about holds shared attribution and program descriptions
// for the wShare server and admin UIs.
package about

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// App identifies which binary is showing the about panel.
type App string

const (
	AppServer App = "server"
	AppAdmin  App = "admin"
)

// Contributor is credited in the About screen.
type Contributor struct {
	Name string
	Note string
	URL  string
}

const (
	ProjectName    = "wShare"
	License        = "MIT"
	OriginalRepo   = "https://github.com/thewhitetulip/wshare"
	ExtendedRepo   = "https://github.com/JohnDovey/wshare"
	OriginalName   = "Suraj Patil"
	OriginalHandle = "thewhitetulip"
	// DefaultWidth is used when the UI does not supply a column width.
	DefaultWidth = 42
)

// Contributors lists original author and known contributors.
func Contributors() []Contributor {
	return []Contributor{
		{
			Name: "Suraj Patil",
			Note: "Original author — created wShare for easy intranet file sharing",
			URL:  "https://github.com/thewhitetulip",
		},
		{
			Name: "ujjwal3067",
			Note: "Contributor — IP address extractor and related fixes",
			URL:  "https://github.com/ujjwal3067",
		},
		{
			Name: "John Dovey",
			Note: "Maintainer of this fork — SQLite catalog, browse UI, admin TUI, ignore rules",
			URL:  "https://github.com/JohnDovey",
		},
	}
}

// ProjectBlurb is a short overview of the project family.
func ProjectBlurb() string {
	return "wShare makes sharing files on a local network simple. " +
		"Originally a lightweight Go tool that served a single file or zip, " +
		"this edition keeps a SQLite catalog of paths, respects .gitignore and robots.txt, " +
		"and offers a browser list plus a terminal admin."
}

// AppBlurb describes one of the two programs.
func AppBlurb(app App) string {
	switch app {
	case AppAdmin:
		return "wshare-admin is a console application for managing the share catalog. " +
			"Browse roots and entries, add or remove paths, clean up missing files, " +
			"and keep the same database the share server uses."
	default:
		return "wshare is the share server. Point it at a file or folder to add it to the catalog, " +
			"then browse and download over the local network. Folders open for navigation; " +
			"check “zip” only when you want an archive. Missing paths appear in amber without a link."
	}
}

// AppTitle is the short title for the about header.
func AppTitle(app App) string {
	switch app {
	case AppAdmin:
		return "wShare Admin"
	default:
		return "wShare Server"
	}
}

// PlainText renders a full about document for the terminal UI.
// width is the content column width in runes (viewport inner width).
func PlainText(app App, width int) string {
	if width < 24 {
		width = DefaultWidth
	}

	var b strings.Builder
	title := AppTitle(app)
	fmt.Fprintf(&b, "%s\n", title)
	ruleLen := utf8.RuneCountInString(title) + 2
	if ruleLen > width {
		ruleLen = width
	}
	fmt.Fprintf(&b, "%s\n\n", strings.Repeat("─", ruleLen))

	b.WriteString("About this program\n")
	b.WriteString(Wrap(AppBlurb(app), width) + "\n\n")

	b.WriteString("About wShare\n")
	b.WriteString(Wrap(ProjectBlurb(), width) + "\n\n")

	b.WriteString("Original project\n")
	b.WriteString(WrapIndent(fmt.Sprintf("%s (@%s)", OriginalName, OriginalHandle), width, "  ") + "\n")
	b.WriteString(WrapIndent(OriginalRepo, width, "  ") + "\n\n")

	b.WriteString("This edition\n")
	b.WriteString(WrapIndent(ExtendedRepo, width, "  ") + "\n\n")

	b.WriteString("Contributors\n")
	for _, c := range Contributors() {
		b.WriteString(WrapIndent("• "+c.Name, width, "  ") + "\n")
		b.WriteString(WrapIndent(c.Note, width, "    ") + "\n")
		if c.URL != "" {
			b.WriteString(WrapIndent(c.URL, width, "    ") + "\n")
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "License: %s\n", License)
	b.WriteString("\n" + Wrap("Press esc or ? to close", width))
	return b.String()
}

// HTMLBody returns inner HTML for the about drawer (already escaped where needed).
func HTMLBody(app App) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<h2>%s</h2>`, esc(AppTitle(app)))
	b.WriteString(`<section><h3>About this program</h3><p>`)
	b.WriteString(esc(AppBlurb(app)))
	b.WriteString(`</p></section>`)
	b.WriteString(`<section><h3>About wShare</h3><p>`)
	b.WriteString(esc(ProjectBlurb()))
	b.WriteString(`</p></section>`)
	b.WriteString(`<section><h3>Original project</h3><p>`)
	fmt.Fprintf(&b, `<strong>%s</strong> (<a href="%s" target="_blank" rel="noopener">@%s</a>)<br>`,
		esc(OriginalName), esc(OriginalRepo), esc(OriginalHandle))
	fmt.Fprintf(&b, `<a class="about-url" href="%s" target="_blank" rel="noopener">%s</a></p></section>`,
		esc(OriginalRepo), esc(OriginalRepo))
	b.WriteString(`<section><h3>This edition</h3><p>`)
	fmt.Fprintf(&b, `<a class="about-url" href="%s" target="_blank" rel="noopener">%s</a></p></section>`,
		esc(ExtendedRepo), esc(ExtendedRepo))
	b.WriteString(`<section><h3>Contributors</h3><ul class="about-contrib">`)
	for _, c := range Contributors() {
		b.WriteString(`<li>`)
		if c.URL != "" {
			fmt.Fprintf(&b, `<a href="%s" target="_blank" rel="noopener"><strong>%s</strong></a>`, esc(c.URL), esc(c.Name))
		} else {
			fmt.Fprintf(&b, `<strong>%s</strong>`, esc(c.Name))
		}
		fmt.Fprintf(&b, `<div class="about-note">%s</div>`, esc(c.Note))
		if c.URL != "" {
			fmt.Fprintf(&b, `<div class="about-url">%s</div>`, esc(c.URL))
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></section>`)
	fmt.Fprintf(&b, `<p class="about-license">License: %s</p>`, esc(License))
	return b.String()
}

func esc(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

// Wrap word-wraps s to the given rune width, breaking long tokens (URLs) as needed.
func Wrap(s string, width int) string {
	return wrapLines(s, width, "")
}

// WrapIndent wraps s and prefixes every line with indent (indent counts toward width).
func WrapIndent(s string, width int, indent string) string {
	return wrapLines(s, width, indent)
}

func wrapLines(s string, width int, indent string) string {
	if width < 12 {
		width = 12
	}
	indentW := utf8.RuneCountInString(indent)
	bodyW := width - indentW
	if bodyW < 8 {
		bodyW = 8
	}

	words := strings.Fields(s)
	if len(words) == 0 {
		if strings.TrimSpace(s) == "" {
			return ""
		}
		// Preserve non-field content by hard-breaking.
		return indent + hardBreak(strings.TrimSpace(s), bodyW, indent, bodyW)
	}

	var lines []string
	var line string
	for _, word := range words {
		// Word longer than a line: flush current, then hard-break the word.
		if utf8.RuneCountInString(word) > bodyW {
			if line != "" {
				lines = append(lines, indent+line)
				line = ""
			}
			lines = append(lines, hardBreakLines(word, bodyW, indent)...)
			continue
		}
		if line == "" {
			line = word
			continue
		}
		if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > bodyW {
			lines = append(lines, indent+line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, indent+line)
	}
	return strings.Join(lines, "\n")
}

func hardBreakLines(s string, bodyW int, indent string) []string {
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		n := bodyW
		if n > len(runes) {
			n = len(runes)
		}
		out = append(out, indent+string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

func hardBreak(s string, bodyW int, indent string, _ int) string {
	return strings.Join(hardBreakLines(s, bodyW, indent), "\n")
}
