package main

import (
	"regexp"
	"sort"
	"strings"
)

// filterState holds the active include/exclude patterns for the current
// browsing session (seeded from the platform config, adjustable in the
// on-device filter menu). Matching is case-insensitive substring.
type filterState struct {
	Include []string
	Exclude []string
}

func newFilterState(p *Platform) *filterState {
	fs := &filterState{}
	if p != nil {
		fs.Include = append(fs.Include, p.Filters.Inclusive...)
		fs.Exclude = append(fs.Exclude, p.Filters.Exclusive...)
	}
	return fs
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// apply returns the entries surviving the filters. If the result would
// be empty, filters are skipped so the full list still shows.
func (fs *filterState) apply(files []FileEntry) []FileEntry {
	if len(fs.Include) == 0 && len(fs.Exclude) == 0 {
		return files
	}
	var out []FileEntry
	for _, f := range files {
		name := f.Shown()
		if len(fs.Include) > 0 {
			hit := false
			for _, p := range fs.Include {
				if containsFold(name, p) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		skip := false
		for _, p := range fs.Exclude {
			if containsFold(name, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		logf("filters would remove every file; showing unfiltered list")
		return files
	}
	return out
}

func (fs *filterState) has(list []string, tag string) bool {
	for _, t := range list {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func remove(list []string, tag string) []string {
	out := list[:0]
	for _, t := range list {
		if !strings.EqualFold(t, tag) {
			out = append(out, t)
		}
	}
	return out
}

// cycle advances a tag through none -> include -> exclude -> none.
func (fs *filterState) cycle(tag string) {
	switch {
	case fs.has(fs.Include, tag):
		fs.Include = remove(fs.Include, tag)
		fs.Exclude = append(fs.Exclude, tag)
	case fs.has(fs.Exclude, tag):
		fs.Exclude = remove(fs.Exclude, tag)
	default:
		fs.Include = append(fs.Include, tag)
	}
}

func (fs *filterState) stateOf(tag string) string {
	if fs.has(fs.Include, tag) {
		return "[+] "
	}
	if fs.has(fs.Exclude, tag) {
		return "[-] "
	}
	return "    "
}

var tagRe = regexp.MustCompile(`[\[(][^\])]*[\])]`)

// extractTags pulls unique tags like "(USA)" or "[b1]" out of filenames.
func extractTags(files []FileEntry) []string {
	seen := map[string]string{} // lowercase -> first-seen casing
	for _, f := range files {
		for _, m := range tagRe.FindAllString(f.Shown(), -1) {
			lc := strings.ToLower(m)
			if _, ok := seen[lc]; !ok {
				seen[lc] = m
			}
		}
	}
	out := make([]string, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// applySearch filters by case-insensitive substring.
func applySearch(files []FileEntry, term string) []FileEntry {
	if term == "" {
		return files
	}
	var out []FileEntry
	for _, f := range files {
		if containsFold(f.Shown(), term) {
			out = append(out, f)
		}
	}
	return out
}
