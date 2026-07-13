package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sdcard = "/mnt/SDCARD"

var (
	resDir   = "res"
	cacheDir = filepath.Join("res", "cache")
	logFile  *os.File
)

func initDirs() {
	os.MkdirAll(cacheDir, 0755)
	logFile, _ = os.Create(filepath.Join(resDir, "app.log"))
}

func logf(format string, args ...any) {
	if logFile != nil {
		fmt.Fprintf(logFile, format+"\n", args...)
	}
}

func humanSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%d KB", b/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// ---------- cache ----------

func cachePath(id string) string {
	// identifiers are [A-Za-z0-9._-] on archive.org; strip anything else
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, id)
	return filepath.Join(cacheDir, safe+".json")
}

func loadCache(id string) []FileEntry {
	data, err := os.ReadFile(cachePath(id))
	if err != nil {
		return nil
	}
	var files []FileEntry
	if json.Unmarshal(data, &files) != nil || len(files) == 0 {
		return nil
	}
	return files
}

func saveCache(id string, files []FileEntry) {
	if data, err := json.Marshal(files); err == nil {
		os.WriteFile(cachePath(id), data, 0644)
	}
}

// ---------- favorites ----------

func favoritesPath() string { return filepath.Join(resDir, "favorites.txt") }

func loadFavorites() []string {
	f, err := os.Open(favoritesPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func addFavorite(id string) {
	for _, f := range loadFavorites() {
		if f == id {
			return
		}
	}
	f, err := os.OpenFile(favoritesPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, id)
}

// ---------- destination ----------

func destFilePath() string { return filepath.Join(resDir, "destination.txt") }

func getDest(cfg *Config) string {
	if data, err := os.ReadFile(destFilePath()); err == nil {
		d := strings.TrimSpace(string(data))
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	if cfg.DefaultDestination != "" {
		return cfg.DefaultDestination
	}
	return filepath.Join(sdcard, "Downloads")
}

func setDest(path string) {
	os.WriteFile(destFilePath(), []byte(path), 0644)
}

// resolveFolder turns a config folder value into an absolute path:
// absolute paths pass through, SD-relative paths get the SD prefix, and
// a bare word is treated as a NextUI system tag ("GBA" matches
// "Roms/Game Boy Advance (GBA)", or a tagless "Roms/GBA" folder).
func resolveFolder(val string) string {
	if val == "" {
		return ""
	}
	if strings.HasPrefix(val, "/") {
		return val
	}
	if strings.Contains(val, "/") || val == "Downloads" {
		return filepath.Join(sdcard, val)
	}
	romsDir := filepath.Join(sdcard, "Roms")
	if entries, err := os.ReadDir(romsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), "("+val+")") {
				return filepath.Join(romsDir, e.Name())
			}
		}
	}
	if st, err := os.Stat(filepath.Join(romsDir, val)); err == nil && st.IsDir() {
		return filepath.Join(romsDir, val)
	}
	return filepath.Join(sdcard, val)
}

// destinationFor picks the download folder for a platform (or nil for
// manual/favorites browsing).
func destinationFor(cfg *Config, p *Platform) string {
	if p != nil {
		if p.LocalDirectory != "" {
			return resolveFolder(p.LocalDirectory)
		}
		if p.SystemTag != "" {
			return resolveFolder(p.SystemTag)
		}
	}
	return getDest(cfg)
}

// ---------- arcade names ----------

func loadArcadeNames() map[string]string {
	f, err := os.Open(filepath.Join(resDir, "arcade_names.tsv"))
	if err != nil {
		return nil
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func applyArcadeNames(files []FileEntry, names map[string]string) {
	if names == nil {
		return
	}
	for i := range files {
		base := files[i].Name
		base = strings.TrimSuffix(base, ".zip")
		base = strings.TrimSuffix(base, ".7z")
		if pretty, ok := names[base]; ok {
			files[i].Display = pretty + " (" + files[i].Name + ")"
		}
	}
}
