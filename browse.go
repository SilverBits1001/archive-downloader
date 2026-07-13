package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"go.uber.org/atomic"
)

// browseItem shows the file list for an archive.org item and handles
// search, filters, refresh, and (multi-)downloads. platform may be nil
// for manual/favorites browsing.
func browseItem(cfg *Config, id string, platform *Platform) {
	id = cleanIdentifier(id)
	if id == "" {
		return
	}

	fromCache := false
	files := loadCache(id)
	if files != nil {
		fromCache = true
	} else {
		var err error
		files, err = loadItemFresh(cfg, id, platform)
		if err != nil {
			showError("Couldn't load item:\n" + id + "\n\n" + friendlyError(err))
			return
		}
	}
	if platform != nil && platform.IsArcade {
		applyArcadeNames(files, loadArcadeNames())
	}
	addFavorite(id)

	fs := newFilterState(platform)
	search := ""
	selectedIndex := 0

	for {
		visible := applySearch(fs.apply(files), search)

		// pinned control rows, then one row per file
		type rowRef struct{ file *FileEntry }
		var rows []rowRef
		var items []gaba.MenuItem

		addControl := func(text string) {
			rows = append(rows, rowRef{})
			items = append(items, gaba.MenuItem{Text: text, NotMultiSelectable: true})
		}
		if search != "" {
			addControl(fmt.Sprintf(">> Search: %s  (%d of %d)", search, len(visible), len(files)))
		} else {
			addControl(fmt.Sprintf(">> Search  (%d files)", len(files)))
		}
		addControl(">> Filters")
		if fromCache {
			addControl(">> Refresh file list  (cached)")
		}
		nControls := len(items)
		for i := range visible {
			f := &visible[i]
			rows = append(rows, rowRef{file: f})
			items = append(items, gaba.MenuItem{
				Text: fmt.Sprintf("%s  [%s]", f.Shown(), humanSize(f.Size)),
			})
		}

		opts := gaba.DefaultListOptions(id, items)
		opts.SelectedIndex = selectedIndex
		opts.EmptyMessage = "No files to show"
		opts.FooterHelpItems = []gaba.FooterHelpItem{
			{ButtonName: "A", HelpText: "Download"},
			{ButtonName: "Select", HelpText: "Multi"},
			{ButtonName: "B", HelpText: "Back"},
		}

		res, err := gaba.List(opts)
		if err != nil {
			if errors.Is(err, gaba.ErrCancelled) {
				return
			}
			logf("list error: %v", err)
			return
		}
		if len(res.Selected) == 0 {
			return
		}
		selectedIndex = res.Selected[0]

		// control rows only act when selected alone
		if len(res.Selected) == 1 && res.Selected[0] < nControls {
			switch res.Selected[0] {
			case 0: // search
				kb, err := gaba.Keyboard(search, "Search files (empty = show all)")
				if err == nil && kb != nil {
					search = kb.Text
					selectedIndex = 0
				}
			case 1: // filters
				filtersMenu(fs, files)
				selectedIndex = 0
			case 2: // refresh
				fresh, err := loadItemFresh(cfg, id, platform)
				if err != nil {
					showError("Couldn't refresh the list.\nStill using the cached copy.")
				} else {
					files = fresh
					if platform != nil && platform.IsArcade {
						applyArcadeNames(files, loadArcadeNames())
					}
					fromCache = false
					selectedIndex = 0
				}
			}
			continue
		}

		// gather chosen files (ignore any control rows in a multi-select)
		var chosen []FileEntry
		for _, idx := range res.Selected {
			if idx >= nControls && rows[idx].file != nil {
				chosen = append(chosen, *rows[idx].file)
			}
		}
		if len(chosen) == 0 {
			continue
		}
		downloadFiles(cfg, id, platform, chosen)
	}
}

// loadItemFresh fetches metadata from the network (with a loading
// screen) and updates the cache.
func loadItemFresh(cfg *Config, id string, platform *Platform) ([]FileEntry, error) {
	return gaba.ProcessMessage(
		"Loading item...\n"+id,
		gaba.ProcessMessageOptions{ShowThemeBackground: true},
		func() ([]FileEntry, error) {
			files, err := fetchMetadata(id, cfg.AuthHeaders())
			if err != nil {
				return nil, err
			}
			saveCache(id, files)
			return files, nil
		})
}

// filtersMenu lets the user cycle tags none -> [+] -> [-] -> none.
func filtersMenu(fs *filterState, files []FileEntry) {
	tags := extractTags(files)
	if len(tags) == 0 {
		showInfo("No filter tags found in these files.")
		return
	}
	selected := 0
	for {
		items := make([]gaba.MenuItem, 0, len(tags)+1)
		items = append(items, gaba.MenuItem{Text: ">> Apply & Return"})
		for _, t := range tags {
			items = append(items, gaba.MenuItem{Text: fs.stateOf(t) + t})
		}
		opts := gaba.DefaultListOptions("Filters (A toggles)", items)
		opts.SelectedIndex = selected
		opts.FooterHelpItems = []gaba.FooterHelpItem{
			{ButtonName: "A", HelpText: "Toggle"},
			{ButtonName: "B", HelpText: "Done"},
		}
		res, err := gaba.List(opts)
		if err != nil || len(res.Selected) == 0 {
			return // B = done, filters already updated in place
		}
		idx := res.Selected[0]
		if idx == 0 {
			return
		}
		fs.cycle(tags[idx-1])
		selected = idx
	}
}

// downloadFiles queues the chosen files through gabagool's download
// manager, then extracts archives where the platform settings allow.
func downloadFiles(cfg *Config, id string, platform *Platform, chosen []FileEntry) {
	dest := destinationFor(cfg, platform)
	if err := os.MkdirAll(dest, 0755); err != nil {
		showError("Can't create destination:\n" + dest)
		return
	}

	var downloads []gaba.Download
	for _, f := range chosen {
		downloads = append(downloads, gaba.Download{
			URL:         downloadURL(id, f.Name),
			Location:    filepath.Join(dest, filepath.Base(f.Name)),
			DisplayName: filepath.Base(f.Shown()),
			Timeout:     2 * time.Hour,
		})
	}

	res, err := gaba.DownloadManager(downloads, cfg.AuthHeaders(), gaba.DownloadManagerOptions{
		MaxConcurrent:       2,
		SkipSSLVerification: insecureMode,
	})
	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			// don't leave partial files behind on cancel
			for _, d := range downloads {
				os.Remove(d.Location)
			}
		} else {
			showError("Download failed:\n" + friendlyError(err))
		}
		return
	}

	// extraction pass
	extracted := 0
	extractErrs := 0
	if platform == nil || platform.ShouldUnzip() {
		var toExtract []gaba.Download
		for _, d := range res.Completed {
			if isArchive(d.Location) {
				toExtract = append(toExtract, d)
			}
		}
		if len(toExtract) > 0 {
			progress := atomic.NewFloat64(0)
			_, _ = gaba.ProcessMessage("Extracting...",
				gaba.ProcessMessageOptions{ShowThemeBackground: true, ShowProgressBar: true, Progress: progress},
				func() (int, error) {
					for i, d := range toExtract {
						if err := extractArchive(d.Location, dest); err != nil {
							extractErrs++
						} else {
							extracted++
						}
						progress.Store(float64(i+1) / float64(len(toExtract)))
					}
					return 0, nil
				})
		}
	}

	msg := fmt.Sprintf("Done!\n%d file(s) downloaded to\n%s", len(res.Completed), filepath.Base(dest))
	if extracted > 0 {
		msg += fmt.Sprintf("\n%d archive(s) extracted", extracted)
	}
	if extractErrs > 0 {
		msg += fmt.Sprintf("\n%d extraction(s) failed (archive kept)", extractErrs)
	}
	if len(res.Failed) > 0 {
		msg = fmt.Sprintf("%d of %d downloads failed.\n%s\n\n%s",
			len(res.Failed), len(downloads),
			friendlyError(res.Failed[0].Error), msg)
		for _, f := range res.Failed {
			logf("download failed: %s: %v", f.Download.URL, f.Error)
			os.Remove(f.Download.Location)
		}
	}
	showInfo(msg)
}
