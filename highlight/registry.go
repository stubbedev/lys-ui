package highlight

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/stubbedev/lys-ui/widget"
)

// Language is a registered highlighter (a GtkSourceView language): its
// id, the other names it answers to (a Markdown fence's hint such as
// "golang" or "yml"), and the file-name patterns it claims.
type Language struct {
	// Name is the language id, lower-case: "go", "json".
	Name string
	// Aliases are further names Lookup accepts.
	Aliases []string
	// Globs are filepath.Match patterns against a file's base name:
	// "*.go", "Cargo.lock".
	Globs []string
	// Interpreters are the programs a script's #! line names to run
	// it: "python3", "bash".
	Interpreters []string
	// Highlighter styles the language.
	Highlighter widget.Highlighter
}

// registry is the language table; a language's index (plus one) is
// its id inside another language's line state (a fenced block).
var registry struct {
	sync.RWMutex
	langs []Language
}

// Register adds lang, or replaces the language of the same name, so an
// application contributes its own languages (or overrides a built-in)
// without forking.
func Register(lang Language) {
	lang.Name = strings.ToLower(lang.Name)
	registry.Lock()
	defer registry.Unlock()
	for i, l := range registry.langs {
		if l.Name == lang.Name {
			registry.langs[i] = lang
			return
		}
	}
	registry.langs = append(registry.langs, lang)
}

// Lookup finds a language's highlighter by name or alias, ignoring
// case.
func Lookup(name string) (widget.Highlighter, bool) {
	if id := languageID(name); id > 0 {
		return languageAt(id), true
	}
	return nil, false
}

// ForFile finds the highlighter whose globs match path's base name.
func ForFile(path string) (widget.Highlighter, bool) {
	base := filepath.Base(path)
	registry.RLock()
	defer registry.RUnlock()
	for _, l := range registry.langs {
		for _, g := range l.Globs {
			if ok, _ := filepath.Match(g, base); ok {
				return l.Highlighter, true
			}
		}
	}
	return nil, false
}

// Languages lists the registered language names, sorted.
func Languages() []string {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]string, len(registry.langs))
	for i, l := range registry.langs {
		out[i] = l.Name
	}
	slices.Sort(out)
	return out
}

// languageID is name's registry id (index plus one), 0 when unknown.
func languageID(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return 0
	}
	registry.RLock()
	defer registry.RUnlock()
	for i, l := range registry.langs {
		if l.Name == name || slices.Contains(l.Aliases, name) {
			return i + 1
		}
	}
	return 0
}

// languageAt is the highlighter of registry id, nil when out of range.
func languageAt(id int) widget.Highlighter {
	registry.RLock()
	defer registry.RUnlock()
	if id < 1 || id > len(registry.langs) {
		return nil
	}
	return registry.langs[id-1].Highlighter
}

// builtinLanguages are the languages this package registers.
var builtinLanguages = []Language{
	{Name: "toml", Globs: []string{"*.toml", "Cargo.lock", "Pipfile"}, Highlighter: TOML{}},
	{Name: "go", Aliases: []string{"golang"}, Globs: []string{"*.go"}, Highlighter: Go{}},
	{Name: "json", Aliases: []string{"jsonc", "geojson"}, Globs: []string{"*.json", "*.geojson", "*.jsonc"}, Highlighter: JSON{}},
	{Name: "yaml", Aliases: []string{"yml"}, Globs: []string{"*.yaml", "*.yml"}, Highlighter: YAML{}},
	{Name: "markdown", Aliases: []string{"md"}, Globs: []string{"*.md", "*.markdown"}, Highlighter: Markdown{}},
	{Name: "c", Aliases: []string{"h"}, Globs: []string{"*.c", "*.h"}, Highlighter: langC},
	{Name: "cpp", Aliases: []string{"c++", "cxx", "cc", "hpp"}, Globs: []string{"*.cpp", "*.cc", "*.cxx", "*.c++", "*.hpp", "*.hh", "*.hxx", "*.h++", "*.ino"}, Highlighter: langCPP},
	{Name: "rust", Aliases: []string{"rs"}, Globs: []string{"*.rs"}, Highlighter: langRust},
	{Name: "javascript", Aliases: []string{"js", "jsx", "mjs", "cjs", "node"}, Globs: []string{"*.js", "*.mjs", "*.cjs", "*.jsx"}, Interpreters: []string{"node", "nodejs", "bun"}, Highlighter: langJavaScript},
	{Name: "typescript", Aliases: []string{"ts", "tsx", "mts", "cts"}, Globs: []string{"*.ts", "*.tsx", "*.mts", "*.cts"}, Interpreters: []string{"deno", "ts-node", "tsx"}, Highlighter: langTypeScript},
	{Name: "java", Globs: []string{"*.java"}, Highlighter: langJava},
	{Name: "csharp", Aliases: []string{"c#", "cs"}, Globs: []string{"*.cs", "*.csx"}, Highlighter: langCSharp},
	{Name: "python", Aliases: []string{"py", "python3", "py3", "gyp"}, Globs: []string{"*.py", "*.pyi", "*.pyw", "SConstruct", "SConscript", "*.gyp"}, Interpreters: []string{"python", "pypy"}, Highlighter: langPython},
	{Name: "sh", Aliases: []string{"bash", "zsh", "shell", "ksh", "dash", "shellscript"}, Globs: []string{"*.sh", "*.bash", "*.zsh", "*.ksh", ".bashrc", ".bash_profile", ".bash_logout", ".zshrc", ".zprofile", ".zshenv", ".profile", "PKGBUILD", "*.ebuild", ".envrc"}, Interpreters: []string{"sh", "bash", "zsh", "dash", "ksh", "mksh", "ash"}, Highlighter: langShell},
	{Name: "nix", Globs: []string{"*.nix"}, Highlighter: langNix},
	{Name: "ruby", Aliases: []string{"rb"}, Globs: []string{"*.rb", "Rakefile", "Gemfile", "*.gemspec", "*.rake"}, Interpreters: []string{"ruby"}, Highlighter: langRuby},
	{Name: "lua", Globs: []string{"*.lua"}, Interpreters: []string{"lua", "luajit"}, Highlighter: langLua},
	{Name: "sql", Aliases: []string{"psql", "sqlite", "mysql", "postgresql"}, Globs: []string{"*.sql"}, Highlighter: langSQL},
	{Name: "makefile", Aliases: []string{"make", "mk"}, Globs: []string{"Makefile", "makefile", "GNUmakefile", "*.mk", "*.mak"}, Interpreters: []string{"make"}, Highlighter: langMake},
	{Name: "dockerfile", Aliases: []string{"docker", "containerfile"}, Globs: []string{"Dockerfile", "Containerfile", "*.dockerfile", "Dockerfile.*"}, Highlighter: langDockerfile},
	{Name: "html", Aliases: []string{"htm", "xhtml"}, Globs: []string{"*.html", "*.htm", "*.xhtml"}, Highlighter: Tags{}},
	{Name: "xml", Aliases: []string{"svg", "xsl", "plist"}, Globs: []string{"*.xml", "*.svg", "*.xsd", "*.xsl", "*.xslt", "*.plist", "*.ui", "*.glade", "*.rss", "*.atom"}, Highlighter: Tags{}},
	{Name: "css", Globs: []string{"*.css"}, Highlighter: CSS{}},
	{Name: "ini", Aliases: []string{"dosini", "conf", "desktop", "systemd", "gitconfig", "editorconfig"}, Globs: []string{"*.ini", "*.cfg", "*.conf", "*.desktop", "*.service", "*.socket", "*.timer", "*.target", "*.mount", "*.path", "*.network", ".gitconfig", ".editorconfig", "*.inf"}, Highlighter: INI{}},
	{Name: "diff", Aliases: []string{"patch", "udiff"}, Globs: []string{"*.diff", "*.patch"}, Highlighter: Diff{}},
}

func init() {
	for _, l := range builtinLanguages {
		Register(l)
	}
}
