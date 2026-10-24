package scheme

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed bundled/*.yaml
var bundledFiles embed.FS

// Bundled lists the slugs of the themes that ship with strig, sorted.
func Bundled() []string {
	entries, err := fs.ReadDir(bundledFiles, "bundled")
	if err != nil {
		return nil // the embedded directory always exists
	}
	var slugs []string
	for _, entry := range entries {
		slugs = append(slugs, strings.TrimSuffix(entry.Name(), ".yaml"))
	}
	slices.Sort(slugs)
	return slugs
}

// Location says where a theme value points: a bundled theme (Slug) or a file (Path).
type Location struct {
	Slug string
	Path string
}

// Locate decides what a `theme:` config value names. A value containing "/"
// or ending in .yaml or .yml is a file: "~" is expanded to the home
// directory and a relative path is taken from configDirectory. Anything else
// is a bundled theme's slug. home is called only when a "~" needs expanding.
func Locate(value, configDirectory string, home func() (string, error)) (Location, error) {
	lower := strings.ToLower(value)
	switch {
	case lower == "terminal":
		return Location{}, errors.New(`theme "terminal" is not supported yet`)
	case strings.HasSuffix(lower, ".xresources"):
		return Location{}, errors.New("theme files in Xresources format are not supported yet")
	case !strings.Contains(value, "/") && !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml"):
		return Location{Slug: value}, nil
	}
	path := value
	if path == "~" || strings.HasPrefix(path, "~/") {
		homeDirectory, err := home()
		if err != nil {
			return Location{}, fmt.Errorf("expand ~ in theme %q: %w", value, err)
		}
		path = filepath.Join(homeDirectory, path[1:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDirectory, path)
	}
	return Location{Path: path}, nil
}

// Load reads the theme at location.
func Load(location Location) (Scheme, error) {
	if location.Path == "" {
		return LoadBundled(location.Slug)
	}
	file, err := os.Open(location.Path)
	if err != nil {
		return Scheme{}, fmt.Errorf("read theme: %w", err)
	}
	defer func() { _ = file.Close() }() // read-only
	// One byte over the limit is enough for Parse to refuse the file.
	data, err := io.ReadAll(io.LimitReader(file, MaximumFileSize+1))
	if err != nil {
		return Scheme{}, fmt.Errorf("read theme %s: %w", location.Path, err)
	}
	return Parse(location.Path, data)
}

// LoadBundled reads a bundled theme by slug.
func LoadBundled(slug string) (Scheme, error) {
	if !slices.Contains(Bundled(), slug) {
		return Scheme{}, fmt.Errorf("unknown theme %q (bundled themes: %s; a theme file path contains \"/\" or ends in .yaml)", slug, strings.Join(Bundled(), ", "))
	}
	data, err := bundledFiles.ReadFile("bundled/" + slug + ".yaml")
	if err != nil {
		return Scheme{}, fmt.Errorf("read bundled theme %q: %w", slug, err)
	}
	return Parse(slug+".yaml", data)
}

// Resolve loads the theme a `theme:` config value names.
func Resolve(value, configDirectory string, home func() (string, error)) (Scheme, error) {
	location, err := Locate(value, configDirectory, home)
	if err != nil {
		return Scheme{}, err
	}
	return Load(location)
}
