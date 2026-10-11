package web

import (
	"database/sql"
	"net/http"
	"os"
	"strings"

	"github.com/tyandl/switchgopher/internal/config"
)

// readConfigFile resolves a "folder" query-param value (a key into
// config.Preferences, e.g. "config_dir") plus a caller-supplied relative
// file path into actual file content. Returns ok=false for anything that
// keeps this from working - an unconfigured/missing folder preference, or a
// file that isn't there - so callers can show a clear "not available"
// message instead of an empty page or a crash. Nothing populates config_dir/
// dated_config_dir itself yet (see resources/configs/README.md) - this is
// deliberately tolerant of that directory just not existing.
func readConfigFile(folderKey, file string) (content string, ok bool) {
	folder, ok := configFolder(folderKey)
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(safeJoin(folder, file))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// configFolder resolves a folder preference key to its directory. Only these
// two preferences name config directories; restricting the keys keeps the
// download and config endpoints from serving files out of any other string
// preference (their "folder" parameter comes from the URL).
func configFolder(key string) (string, bool) {
	if key != "config_dir" && key != "dated_config_dir" {
		return "", false
	}
	folder, ok := config.Preferences[key].(string)
	if !ok || folder == "" {
		return "", false
	}
	return folder, true
}

// configHandler renders /campus/switches/configs/config/ for one saved config
// file. Query: switch, folder (config_dir or dated_config_dir) and file.
func configHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Config",
		"Version": Version,
	}
	result, ok := req.URL.Query()["switch"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	p["Device"] = result[0]

	result, ok = req.URL.Query()["folder"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	folder := result[0]
	p["Folder"] = folder

	result, ok = req.URL.Query()["file"]
	if !ok || result[0] == "" {
		redirectHome(w, req)
		return
	}
	file := result[0]
	p["File"] = file

	if content, ok := readConfigFile(folder, file); ok {
		p["Content"] = content
	} else {
		p["Content"] = "(configuration not available)"
	}

	if err := templates.ExecuteTemplate(w, "config.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// diffLine is one line of a diffLines result.
type diffLine struct {
	Kind string // "same", "add", or "remove"
	Text string
}

// diffLines computes a minimal line-based diff via the standard LCS
// (longest common subsequence) dynamic-programming approach. O(n*m) time
// and space - fine for config-file-sized input, not meant for huge files.
func diffLines(a, b []string) []diffLine {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var out []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, diffLine{"same", a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffLine{"remove", a[i]})
			i++
		default:
			out = append(out, diffLine{"add", b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffLine{"remove", a[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffLine{"add", b[j]})
	}
	return out
}

// diffHandler compares two config snapshots, named the same way
// configHandler names one - a "folder" preference key plus a relative file
// path, once for each side ("from"/"fromFolder" and "to"/"toFolder").
func diffHandler(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	p := map[string]Any{
		"Title":   "Diff",
		"Version": Version,
	}
	q := req.URL.Query()
	device, fromFolder, from, toFolder, to := q.Get("switch"), q.Get("fromFolder"), q.Get("from"), q.Get("toFolder"), q.Get("to")
	if device == "" || from == "" || to == "" {
		redirectHome(w, req)
		return
	}
	p["Device"] = device

	fromContent, fromOK := readConfigFile(fromFolder, from)
	toContent, toOK := readConfigFile(toFolder, to)
	if !fromOK || !toOK {
		p["Unavailable"] = true
	} else {
		p["Lines"] = diffLines(strings.Split(fromContent, "\n"), strings.Split(toContent, "\n"))
	}

	if err := templates.ExecuteTemplate(w, "diff.tmpl", &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
