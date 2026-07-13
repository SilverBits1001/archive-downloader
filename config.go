package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Auth struct {
	Access       string `yaml:"access,omitempty"`
	Secret       string `yaml:"secret,omitempty"`
	LoggedInUser string `yaml:"logged_in_user,omitempty"`
	LoggedInSig  string `yaml:"logged_in_sig,omitempty"`
}

type Filters struct {
	Inclusive []string `yaml:"inclusive_filters,omitempty"`
	Exclusive []string `yaml:"exclusive_filters,omitempty"`
}

type Platform struct {
	PlatformName   string  `yaml:"platform_name"`
	Identifier     string  `yaml:"identifier"`
	SystemTag      string  `yaml:"system_tag,omitempty"`
	LocalDirectory string  `yaml:"local_directory,omitempty"`
	IsArcade       bool    `yaml:"is_arcade,omitempty"`
	Unzip          *bool   `yaml:"unzip,omitempty"` // nil = default (extract)
	Filters        Filters `yaml:"filters,omitempty"`
}

type Host struct {
	DisplayName string     `yaml:"display_name"`
	Platforms   []Platform `yaml:"platforms"`
}

type Config struct {
	Auth               Auth   `yaml:"auth,omitempty"`
	DefaultDestination string `yaml:"default_destination,omitempty"`
	Hosts              []Host `yaml:"hosts"`
}

func (p *Platform) ShouldUnzip() bool {
	if p.IsArcade {
		return false // arcade sets must stay zipped
	}
	if p.Unzip == nil {
		return true
	}
	return *p.Unzip
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// saveConfig rewrites config.yml. Note: comments in the YAML are not
// preserved on save; the README documents every option instead.
func saveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// AuthHeaders returns the HTTP headers for archive.org auth, if configured.
func (c *Config) AuthHeaders() map[string]string {
	h := map[string]string{}
	if c.Auth.Access != "" && c.Auth.Secret != "" {
		h["Authorization"] = "LOW " + c.Auth.Access + ":" + c.Auth.Secret
	}
	if c.Auth.LoggedInUser != "" && c.Auth.LoggedInSig != "" {
		h["Cookie"] = "logged-in-user=" + c.Auth.LoggedInUser + "; logged-in-sig=" + c.Auth.LoggedInSig
	}
	return h
}
