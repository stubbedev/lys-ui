// Package recentfiles tracks recently used files in the XDG
// recently-used.xbel format (TOML-free, XML as the spec says), so lys-ui
// apps share the recents list with the desktop's other citizens. The
// manager keeps the list bounded, most-recent first, and writes
// atomically; a missing or corrupt file starts an empty list.
package recentfiles

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/atomicfile"

	"github.com/stubbedev/lys-ui/transfer"
)

// Entry is one recently used file: its path (decoded from the file://
// URI on disk), the owning application's name, and when it was last
// used.
type Entry struct {
	Path string
	App  string
	When time.Time
}

// maxEntries bounds the list: the spec's consumers show a handful, and
// an unbounded file grows forever.
const maxEntries = 100

// Manager reads and writes one recents list. The zero value is usable
// and answers from an empty list until a Load or Add names a file.
type Manager struct {
	path    string
	entries []Entry
}

// DefaultPath is the spec's location under the user data directory,
// honoring XDG_DATA_HOME.
func DefaultPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "recently-used.xbel"
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "recently-used.xbel")
}

// New returns the manager for the given xbel path (DefaultPath for the
// desktop's shared list).
func New(path string) *Manager {
	return &Manager{path: path}
}

// List returns the entries, most recent first.
func (m *Manager) List() []Entry {
	out := make([]Entry, len(m.entries))
	copy(out, m.entries)
	return out
}

// Add moves path to the front of the list under app, stamps it now,
// and persists. Errors persisting are reported once here; a failed
// write never loses the in-memory list.
func (m *Manager) Add(path, app string) error {
	path = filepath.Clean(path)
	entry := Entry{Path: path, App: app, When: time.Now()}
	rest := make([]Entry, 0, len(m.entries))
	for _, e := range m.entries {
		if e.Path != path {
			rest = append(rest, e)
		}
	}
	m.entries = append([]Entry{entry}, rest...)
	if len(m.entries) > maxEntries {
		m.entries = m.entries[:maxEntries]
	}
	return m.Save()
}

// Remove drops path from the list and persists.
func (m *Manager) Remove(path string) error {
	path = filepath.Clean(path)
	m.entries = slices.DeleteFunc(m.entries, func(e Entry) bool {
		return e.Path == path
	})
	return m.Save()
}

// Load reads the xbel file; a missing file is an empty list, a corrupt
// one is an empty list plus the error (the next Add rewrites it
// clean). Entries whose URIs are not local file:// paths are skipped.
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.entries = nil
			return nil
		}
		return err
	}
	var doc xbel
	if err := xml.Unmarshal(data, &doc); err != nil {
		m.entries = nil
		return err
	}
	m.entries = m.entries[:0]
	for _, b := range doc.Bookmarks {
		path, ok := transfer.URIPath(b.Href)
		if !ok {
			continue
		}
		when, err := time.Parse(time.RFC3339, b.Modified)
		if err != nil {
			when = time.Time{}
		}
		app := ""
		if len(b.Infos) > 0 && len(b.Infos[0].Metadatas) > 0 {
			app = b.Infos[0].Metadatas[0].App.Name
		}
		m.entries = append(m.entries, Entry{Path: path, App: app, When: when})
	}
	slices.SortStableFunc(m.entries, func(a, b Entry) int {
		if a.When.After(b.When) {
			return -1
		}
		if a.When.Before(b.When) {
			return 1
		}
		// Ties keep the file's order, which the writer emits newest
		// first - RFC3339 stamps have second resolution, so a burst of
		// adds lands tied and the stable sort preserves the truth.
		return 0
	})
	if len(m.entries) > maxEntries {
		m.entries = m.entries[:maxEntries]
	}
	return nil
}

// Save writes the list atomically (temp file + rename); an empty list
// writes an empty xbel, not a deletion, so a shared desktop list does
// not flap between present and absent.
func (m *Manager) Save() error {
	doc := xbel{Version: "1.0", XMLNSBookmark: "http://www.freedesktop.org/standards/desktop-bookmarks"}
	for _, e := range m.entries {
		doc.Bookmarks = append(doc.Bookmarks, xbelBookmark{
			Href:     transfer.FileURI(e.Path),
			Added:    e.When.Format(time.RFC3339),
			Modified: e.When.Format(time.RFC3339),
			Visited:  e.When.Format(time.RFC3339),
			Infos: []xbelInfo{{
				Metadatas: []xbelMetadata{{
					Owner: "lys-ui",
					App:   xbelApp{Name: e.App},
				}},
			}},
		})
	}
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	body := append([]byte(xml.Header), data...)
	body = append(body, '\n')
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicfile.WriteFile(m.path, body, 0o600)
}

// xbel is the on-disk document; only the fields lys-ui writes and reads
// are modeled, unknown content is dropped on rewrite.
type xbel struct {
	XMLName       xml.Name       `xml:"xbel"`
	Version       string         `xml:"version,attr"`
	XMLNSBookmark string         `xml:"xmlns:bookmark,attr"`
	Bookmarks     []xbelBookmark `xml:"bookmark"`
}

type xbelBookmark struct {
	Href     string     `xml:"href,attr"`
	Added    string     `xml:"added,attr,omitempty"`
	Modified string     `xml:"modified,attr,omitempty"`
	Visited  string     `xml:"visited,attr,omitempty"`
	Infos    []xbelInfo `xml:"info"`
}

type xbelInfo struct {
	Metadatas []xbelMetadata `xml:"metadata"`
}

type xbelMetadata struct {
	Owner string  `xml:"owner,attr"`
	App   xbelApp `xml:"app"`
}

type xbelApp struct {
	Name string `xml:"name,attr,omitempty"`
	Exec string `xml:"exec,attr,omitempty"`
}
