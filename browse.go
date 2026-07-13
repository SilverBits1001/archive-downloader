package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/constants"
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
	multiMode := false // toggled by Select; drives which footer/controls show

	for {
		visible := applySearch(fs.apply(files), search)
		filtered := len(visible) != len(files)

		// one optional pinned row (refresh), then one row per file.
		// the refresh row is hidden in multi-select so indices stay clean.
		var items []gaba.MenuItem
		nControls := 0
		if fromCache && !multiMode {
			items = append(items, gaba.MenuItem{Text: ">> Refresh file list  (cached)", NotMultiSelectable: true})
			nControls = 1
		}
		for i := range visible {
			f := &visible[i]
			items = append(items, gaba.MenuItem{
				Text: fmt.Sprintf("%s  [%s]", f.Shown(), humanSize(f.Size)),
			})
		}

		title := id
		if multiMode {
			title = "Select files - " + id
		} else if filtered {
			title = fmt.Sprintf("%s  (%d of %d)", id, len(visible), len(files))
		}
		opts := gaba.DefaultListOptions(title, items)
		opts.SelectedIndex = selectedIndex
		// Select always toggles multi-select mode (tertiary action returns
		// control here so we can flip modes and redraw the right footer).
		opts.TertiaryActionButton = constants.VirtualButtonSelect
		if multiMode {
			opts.InitialMultiSelectMode = true
			opts.MultiSelectConfirmButton = constants.VirtualButtonStart
			opts.SelectAllButton = constants.VirtualButtonL1
			opts.DeselectAllButton = constants.VirtualButtonR1
			opts.EmptyMessage = "No files"
			opts.FooterHelpItems = []gaba.FooterHelpItem{
				{ButtonName: "A", HelpText: "Select"},
				{ButtonName: "L1/R1", HelpText: "All/None"},
				{ButtonName: "Start", HelpText: "Download"},
				{ButtonName: "Select", HelpText: "Exit multi"},
				{ButtonName: "B", HelpText: "Back"},
			}
		} else {
			// X opens filters, Y opens search
			opts.ActionButton = constants.VirtualButtonX
			opts.SecondaryActionButton = constants.VirtualButtonY
			opts.EmptyMessage = "No files match - press Y to change search"
			opts.FooterHelpItems = []gaba.FooterHelpItem{
				{ButtonName: "A", HelpText: "Download"},
				{ButtonName: "X", HelpText: "Filters"},
				{ButtonName: "Y", HelpText: "Search"},
				{ButtonName: "Select", HelpText: "Multi"},
				{ButtonName: "B", HelpText: "Back"},
			}
		}

		res, err := gaba.List(opts)
		if err != nil {
			if errors.Is(err, gaba.ErrCancelled) { // B
				if multiMode {
					multiMode = false // back one level: multi -> normal
					selectedIndex = 0
					continue
				}
				return // normal -> leave the item
			}
			logf("list error: %v", err)
			return
		}

		switch res.Action {
		case gaba.ListActionTertiaryTriggered: // Select = toggle multi-select
			multiMode = !multiMode
			selectedIndex = 0
			continue
		case gaba.ListActionTriggered: // X = filters (normal mode only)
			filtersMenu(fs, files)
			selectedIndex = 0
			continue
		case gaba.ListActionSecondaryTriggered: // Y = search (normal mode only)
			kb, kerr := gaba.Keyboard(search, "Search files (empty = show all)")
			if kerr == nil && kb != nil {
				search = kb.Text
				selectedIndex = 0
			}
			continue
		}

		if len(res.Selected) == 0 {
			continue
		}
		selectedIndex = res.Selected[0]

		// refresh row acts only when selected alone (normal mode)
		if nControls == 1 && len(res.Selected) == 1 && res.Selected[0] == 0 {
			fresh, ferr := loadItemFresh(cfg, id, platform)
			if ferr != nil {
				showError("Couldn't refresh the list.\nStill using the cached copy.")
			} else {
				files = fresh
				if platform != nil && platform.IsArcade {
					applyArcadeNames(files, loadArcadeNames())
				}
				fromCache = false
				selectedIndex = 0
			}
			continue
		}

		// gather chosen files (skip the refresh row)
		var chosen []FileEntry
		for _, idx := range res.Selected {
			fi := idx - nControls
			if fi >= 0 && fi < len(visible) {
				chosen = append(chosen, visible[fi])
			}
		}
		if len(chosen) == 0 {
			continue
		}
		downloadFiles(cfg, id, platform, chosen)
		multiMode = false // return to normal browsing after a batch
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
// Config-declared filters show up as suggestions, OFF until toggled.
func filtersMenu(fs *filterState, files []FileEntry) {
	tags := menuTags(fs, files)
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

// pickKeepers runs when one archive extracted several unrelated files
// (usually multiple versions of a game): the user ticks what to KEEP
// and everything else from that extraction is removed. Cancelling, or
// confirming with nothing ticked, keeps everything — deletion never
// happens by accident, and only files from this extraction are touched.
// Keeping a .cue/.m3u/.gdi playlist automatically keeps the data files
// it references.
func pickKeepers(dest string, extracted []string) {
	items := make([]gaba.MenuItem, len(extracted))
	for i, p := range extracted {
		rel, rerr := filepath.Rel(dest, p)
		if rerr != nil {
			rel = filepath.Base(p)
		}
		var size int64
		if st, serr := os.Stat(p); serr == nil {
			size = st.Size()
		}
		items[i] = gaba.MenuItem{Text: fmt.Sprintf("%s  [%s]", rel, humanSize(size))}
	}
	opts := gaba.DefaultListOptions("Archive had multiple files - tick what to KEEP", items)
	// permanently multi-select: do NOT bind Select to a toggle here, or
	// the user could turn the checkboxes off and be unable to pick.
	opts.InitialMultiSelectMode = true
	opts.MultiSelectConfirmButton = constants.VirtualButtonStart
	opts.SelectAllButton = constants.VirtualButtonL1
	opts.DeselectAllButton = constants.VirtualButtonR1
	opts.FooterHelpItems = []gaba.FooterHelpItem{
		{ButtonName: "A", HelpText: "Tick"},
		{ButtonName: "L1/R1", HelpText: "All/None"},
		{ButtonName: "Start", HelpText: "Keep ticked, trash rest"},
		{ButtonName: "B", HelpText: "Keep all"},
	}
	res, err := gaba.List(opts)
	if err != nil || len(res.Selected) == 0 {
		return // keep everything
	}

	keep := map[string]bool{}
	var addKeep func(p string)
	addKeep = func(p string) {
		if keep[p] {
			return
		}
		keep[p] = true
		for _, ref := range referencedFiles(p) {
			rp := filepath.Join(filepath.Dir(p), ref)
			for _, e := range extracted {
				if strings.EqualFold(e, rp) {
					addKeep(e)
				}
			}
		}
	}
	for _, idx := range res.Selected {
		if idx >= 0 && idx < len(extracted) {
			addKeep(extracted[idx])
		}
	}

	removed := 0
	for _, p := range extracted {
		if !keep[p] {
			if os.Remove(p) == nil {
				removed++
			}
		}
	}
	logf("keeper pick: kept %d, removed %d", len(keep), removed)
	if removed > 0 {
		showInfo(fmt.Sprintf("Kept %d file(s),\nremoved %d.", len(keep), removed))
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
	var multiGroups [][]string // archives that produced several unrelated files
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
						files, xerr := extractArchive(d.Location, dest)
						if xerr != nil {
							extractErrs++
						} else {
							extracted++
							// several files with different names = probably
							// multiple versions; offer to pick keepers.
							// same-base groups (cue+bin etc.) are one game.
							if len(files) > 1 && !allSameBase(files) {
								multiGroups = append(multiGroups, files)
							}
						}
						progress.Store(float64(i+1) / float64(len(toExtract)))
					}
					return 0, nil
				})
		}
	}
	for _, g := range multiGroups {
		pickKeepers(dest, g)
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
