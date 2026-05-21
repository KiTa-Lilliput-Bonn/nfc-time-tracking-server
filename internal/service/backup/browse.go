package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// BrowseDirEntry is one directory in a browse listing.
type BrowseDirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// BrowseResult is returned by the admin backup browse API.
type BrowseResult struct {
	Path    string           `json:"path"`
	Parent  string           `json:"parent"`
	Entries []BrowseDirEntry `json:"entries"`
}

// BrowseRoots lists filesystem roots (drives on Windows, "/" on Unix).
func BrowseRoots() (BrowseResult, error) {
	if runtime.GOOS == "windows" {
		var entries []BrowseDirEntry
		for _, letter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
			drive := string(letter) + `:\`
			if _, err := os.Stat(drive); err == nil {
				entries = append(entries, BrowseDirEntry{Name: drive, Path: drive, IsDir: true})
			}
		}
		return BrowseResult{Path: "", Parent: "", Entries: entries}, nil
	}
	return BrowseDirectoriesAt("/")
}

// BrowseDirectories lists child directories at pathQuery, or the default start directory when empty.
func (s *Service) BrowseDirectories(ctx context.Context, pathQuery string) (BrowseResult, error) {
	pathQuery = strings.TrimSpace(pathQuery)
	if pathQuery == "" {
		start, err := s.defaultBrowsePath(ctx)
		if err != nil {
			return BrowseResult{}, err
		}
		if start == "" {
			return BrowseRoots()
		}
		pathQuery = start
	}
	cleaned, err := cleanAbsolutePath(pathQuery)
	if err != nil {
		return BrowseResult{}, err
	}
	return BrowseDirectoriesAt(cleaned)
}

func (s *Service) defaultBrowsePath(ctx context.Context) (string, error) {
	if s.Settings != nil {
		p, err := s.Settings.Get(ctx, SettingTargetPath)
		if err == nil {
			p = strings.TrimSpace(p)
			if p != "" {
				if cleaned, err := cleanAbsolutePath(p); err == nil {
					if st, err := os.Stat(cleaned); err == nil && st.IsDir() {
						return cleaned, nil
					}
					return filepath.Dir(cleaned), nil
				}
			}
		}
	}
	if s.DatabasePath != "" {
		dir := filepath.Dir(s.DatabasePath)
		if cleaned, err := cleanAbsolutePath(dir); err == nil {
			if st, err := os.Stat(cleaned); err == nil && st.IsDir() {
				return cleaned, nil
			}
		}
	}
	if runtime.GOOS == "windows" {
		return "", nil
	}
	return "/", nil
}

func cleanAbsolutePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute")
	}
	cleaned := filepath.Clean(p)
	if runtime.GOOS == "windows" {
		if len(cleaned) == 2 && cleaned[1] == ':' {
			cleaned += `\`
		}
	}
	if cleaned == "." {
		return "", fmt.Errorf("invalid path")
	}
	return cleaned, nil
}

// BrowseDirectoriesAt lists immediate child directories of dir (dir must be absolute and clean).
func BrowseDirectoriesAt(dir string) (BrowseResult, error) {
	dir, err := cleanAbsolutePath(dir)
	if err != nil {
		return BrowseResult{}, err
	}
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return BrowseResult{}, fmt.Errorf("directory does not exist")
		}
		return BrowseResult{}, fmt.Errorf("cannot access directory: %w", err)
	}
	if !st.IsDir() {
		return BrowseResult{}, fmt.Errorf("not a directory")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return BrowseResult{}, fmt.Errorf("cannot read directory: %w", err)
	}
	var dirs []BrowseDirEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		child := filepath.Join(dir, name)
		dirs = append(dirs, BrowseDirEntry{Name: name, Path: child, IsDir: true})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := browseParent(dir)
	return BrowseResult{
		Path:    dir,
		Parent:  parent,
		Entries: dirs,
	}, nil
}

func browseParent(dir string) string {
	if runtime.GOOS == "windows" {
		v := filepath.VolumeName(dir)
		if v != "" {
			root := v + `\`
			if strings.EqualFold(filepath.Clean(dir), root) {
				return ""
			}
		}
	} else if dir == "/" {
		return ""
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return ""
	}
	return parent
}
