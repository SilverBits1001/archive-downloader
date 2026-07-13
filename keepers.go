package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func baseNoExt(p string) string {
	b := filepath.Base(p)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// allSameBase reports whether every path shares one base filename —
// e.g. Game.cue + Game.bin: one game in multiple parts, never prompt.
func allSameBase(paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	first := strings.ToLower(baseNoExt(paths[0]))
	for _, p := range paths[1:] {
		if strings.ToLower(baseNoExt(p)) != first {
			return false
		}
	}
	return true
}

// referencedFiles parses cue/m3u/gdi playlists for the filenames they
// point at, so keeping a playlist also keeps its data tracks.
func referencedFiles(path string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".cue" && ext != ".m3u" && ext != ".gdi" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch ext {
		case ".cue":
			// FILE "Game (Track 1).bin" BINARY
			if strings.HasPrefix(strings.ToUpper(line), "FILE ") {
				if i := strings.Index(line, `"`); i >= 0 {
					rest := line[i+1:]
					if j := strings.Index(rest, `"`); j >= 0 {
						out = append(out, rest[:j])
					}
				}
			}
		case ".m3u":
			if line != "" && !strings.HasPrefix(line, "#") {
				out = append(out, line)
			}
		case ".gdi":
			for _, fld := range strings.Fields(line) {
				fld = strings.Trim(fld, `"`)
				if strings.Contains(fld, ".") {
					out = append(out, fld)
				}
			}
		}
	}
	return out
}
