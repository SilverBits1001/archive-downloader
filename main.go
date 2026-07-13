package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

const configPath = "config.yml"

func showError(msg string) {
	_, _ = gaba.ConfirmationMessage(msg,
		[]gaba.FooterHelpItem{{ButtonName: "A", HelpText: "OK"}},
		gaba.MessageOptions{})
}

func showInfo(msg string) { showError(msg) }

func friendlyError(err error) string {
	var hs *httpStatusError
	if errors.As(err, &hs) {
		switch hs.Code {
		case 401, 403:
			return fmt.Sprintf("Access denied (HTTP %d).\nThis item may require signing in -\nadd your login under auth: in config.yml", hs.Code)
		case 404:
			return "Not found on server (404)."
		default:
			return hs.Error()
		}
	}
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}

func main() {
	gaba.Init(gaba.Options{
		WindowTitle:    "Archive Downloader",
		ShowBackground: true,
		IsNextUI:       true,
		LogPath:        "res",
		LogFilename:    "gabagool.log",
	})
	defer gaba.Close()

	initDirs()
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()
	initHTTP(filepath.Join(resDir, "cacert.pem"))

	cfg, err := loadConfig(configPath)
	if err != nil {
		cfg = &Config{}
		logf("config load failed: %v", err)
	}

	ok, _ := gaba.ProcessMessage("Archive Downloader\n\nChecking connection...",
		gaba.ProcessMessageOptions{ShowThemeBackground: true},
		func() (bool, error) {
			return checkNetwork() == nil, nil
		})
	if !ok {
		showError("No internet connection.\n\nConnect to WiFi in NextUI Settings,\nthen relaunch.")
		return
	}
	if insecureMode {
		showInfo("Note: TLS verification unavailable\n(device clock may be wrong).\nContinuing anyway...")
	}

	mainMenu(cfg)
}

// flatPlatform pairs a platform with its host indices for config writes.
type flatPlatform struct {
	Host, Index int
	P           *Platform
}

func flattenPlatforms(cfg *Config) []flatPlatform {
	var out []flatPlatform
	for h := range cfg.Hosts {
		for p := range cfg.Hosts[h].Platforms {
			out = append(out, flatPlatform{Host: h, Index: p, P: &cfg.Hosts[h].Platforms[p]})
		}
	}
	return out
}

func mainMenu(cfg *Config) {
	selected := 0
	for {
		plats := flattenPlatforms(cfg)
		items := []gaba.MenuItem{
			{Text: ">> Enter identifier"},
			{Text: ">> Favorites"},
			{Text: ">> Destination: " + strings.TrimPrefix(getDest(cfg), sdcard+"/")},
		}
		nControls := len(items)
		for _, fp := range plats {
			items = append(items, gaba.MenuItem{Text: fp.P.PlatformName})
		}

		opts := gaba.DefaultListOptions("Archive Downloader", items)
		opts.SelectedIndex = selected
		opts.FooterHelpItems = []gaba.FooterHelpItem{
			{ButtonName: "A", HelpText: "Select"},
			{ButtonName: "B", HelpText: "Quit"},
		}
		res, err := gaba.List(opts)
		if err != nil || len(res.Selected) == 0 {
			return
		}
		selected = res.Selected[0]

		switch {
		case selected == 0:
			kb, err := gaba.Keyboard("", "Item identifier (from archive.org/details/...)")
			if err == nil && kb != nil && strings.TrimSpace(kb.Text) != "" {
				browseItem(cfg, kb.Text, nil)
			}
		case selected == 1:
			favoritesMenu(cfg)
		case selected == 2:
			destinationMenu(cfg)
		default:
			platformMenu(cfg, plats[selected-nControls])
		}
	}
}

func favoritesMenu(cfg *Config) {
	favs := loadFavorites()
	if len(favs) == 0 {
		showInfo("No favorites yet.\nBrowse an item first and it\nwill be saved here.")
		return
	}
	items := make([]gaba.MenuItem, len(favs))
	for i, f := range favs {
		items[i] = gaba.MenuItem{Text: f}
	}
	res, err := gaba.List(gaba.DefaultListOptions("Favorites", items))
	if err != nil || len(res.Selected) == 0 {
		return
	}
	browseItem(cfg, favs[res.Selected[0]], nil)
}

func destinationMenu(cfg *Config) {
	type dst struct{ label, path string }
	dests := []dst{{"Downloads", filepath.Join(sdcard, "Downloads")}}
	romsDir := filepath.Join(sdcard, "Roms")
	if entries, err := os.ReadDir(romsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dests = append(dests, dst{"Roms/" + e.Name(), filepath.Join(romsDir, e.Name())})
			}
		}
	}
	items := make([]gaba.MenuItem, len(dests))
	for i, d := range dests {
		items[i] = gaba.MenuItem{Text: d.label}
	}
	res, err := gaba.List(gaba.DefaultListOptions("Save downloads to...", items))
	if err != nil || len(res.Selected) == 0 {
		return
	}
	d := dests[res.Selected[0]]
	os.MkdirAll(d.path, 0755)
	setDest(d.path)
	showInfo("Destination set:\n" + d.label)
}

func platformMenu(cfg *Config, fp flatPlatform) {
	for {
		p := fp.P
		tagLabel := "Assign System Tag"
		if p.SystemTag != "" {
			tagLabel = fmt.Sprintf("Assign System Tag (%s)", p.SystemTag)
		}
		uzLabel := "Compressed files: Extract after download"
		if p.IsArcade {
			uzLabel = "Compressed files: Kept (arcade)"
		} else if !p.ShouldUnzip() {
			uzLabel = "Compressed files: Keep as-is"
		}
		items := []gaba.MenuItem{
			{Text: "Browse Files"},
			{Text: tagLabel},
			{Text: uzLabel},
		}
		res, err := gaba.List(gaba.DefaultListOptions(p.PlatformName, items))
		if err != nil || len(res.Selected) == 0 {
			return
		}
		switch res.Selected[0] {
		case 0:
			if cleanIdentifier(p.Identifier) == "" {
				showError("No identifier set for\n" + p.PlatformName + " in config.yml.")
				continue
			}
			browseItem(cfg, p.Identifier, p)
		case 1:
			assignTag(cfg, fp)
		case 2:
			if p.IsArcade {
				showInfo("Arcade sets must stay zipped\nto work, so they are never extracted.")
				continue
			}
			v := !p.ShouldUnzip()
			p.Unzip = &v
			if err := saveConfig(configPath, cfg); err != nil {
				showError("Couldn't save config.yml")
			} else if v {
				showInfo("Compressed files will now be\nEXTRACTED after download.")
			} else {
				showInfo("Compressed files will now be\nKEPT as downloaded.")
			}
		}
	}
}

func assignTag(cfg *Config, fp flatPlatform) {
	romsDir := filepath.Join(sdcard, "Roms")
	entries, err := os.ReadDir(romsDir)
	if err != nil || len(entries) == 0 {
		showInfo("No folders found in SDCARD/Roms.")
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	items := make([]gaba.MenuItem, len(names))
	for i, n := range names {
		items[i] = gaba.MenuItem{Text: n}
	}
	res, err := gaba.List(gaba.DefaultListOptions("Select System Tag", items))
	if err != nil || len(res.Selected) == 0 {
		return
	}
	folder := names[res.Selected[0]]
	tag := folder
	if i := strings.LastIndex(folder, "("); i >= 0 && strings.HasSuffix(folder, ")") {
		tag = folder[i+1 : len(folder)-1]
	}
	fp.P.SystemTag = tag
	if err := saveConfig(configPath, cfg); err != nil {
		showError("Couldn't save config.yml")
		return
	}
	showInfo("Assigned tag:\n" + tag)
}
