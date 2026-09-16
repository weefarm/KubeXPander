// Package config loads and validates the user's slug map.
//
// The config file lives at ~/.config/kxp/slugs.yaml by default and maps
// short names (3-8 chars) to Kubernetes namespaces, plus optional "filtered"
// slugs that grep the default pod listing (the kcil/kenv pattern).
//
// Terminology: a "slug" is a short namespace alias. `kclo` dispatches to the
// `cloudflare` namespace — `k` is the prefix, `clo` is the slug.
//
// The `k` prefix is constant; the slug is the namespace alias it dispatches to.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// SlugNameRange is the default allowed length range for slug names.
// 3-8 chars is the sweet spot — short enough to type fast, long enough
// to be memorable and unique.
const (
	SlugNameMin = 3
	SlugNameMax = 8
)

// slugNameRe restricts slug names to lowercase letters and digits,
// keeping them shell-safe and tab-completion friendly.
var slugNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// FilteredSlug is a namespace slug that applies a grep filter to the
// default pod listing (and its -o wide variant), while passing everything else
// (rmf, delete, get deploy, ...) straight through to kubectl unfiltered.
//
// Example: cil -> { ns: kube-system, grep: cilium }
type FilteredSlug struct {
	NS   string `yaml:"ns"`
	Grep string `yaml:"grep"`
}

// Config is the parsed slug map.
type Config struct {
	// Slugs maps slug name -> namespace name.
	//   clo: cloudflare
	//   sys: kube-system
	Slugs map[string]string `yaml:"slugs"`

	// Filtered maps slug name -> filtered slug spec.
	//   cil: { ns: kube-system, grep: cilium }
	Filtered map[string]FilteredSlug `yaml:"filtered"`

	// AllSlug is the name of the all-namespaces slug (default "all").
	AllSlug string `yaml:"allSlug"`

	// AllowShorter disables the 3-char minimum length check.
	AllowShorter bool `yaml:"allowShorter"`

	// AllowLonger disables the 8-char maximum length check.
	AllowLonger bool `yaml:"allowLonger"`
}

// DefaultPath returns the default config file location:
// $XDG_CONFIG_HOME/kxp/slugs.yaml, falling back to ~/.config/kxp/slugs.yaml.
func DefaultPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kxp", "slugs.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	return filepath.Join(home, ".config", "kxp", "slugs.yaml"), nil
}

// Load reads and parses the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if c.AllSlug == "" {
		c.AllSlug = "all"
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks that slug names are within the 3-8 char range (unless
// overridden), match the allowed character set, and don't collide between
// the slugs and filtered maps.
func (c *Config) Validate() error {
	min, max := SlugNameMin, SlugNameMax
	if c.AllowShorter {
		min = 1
	}
	if c.AllowLonger {
		max = 0 // unlimited
	}

	seen := make(map[string]string, len(c.Slugs)+len(c.Filtered))

	for name, ns := range c.Slugs {
		if err := validateName(name, min, max); err != nil {
			return err
		}
		if ns == "" {
			return fmt.Errorf("slug %q maps to empty namespace", name)
		}
		if prev, ok := seen[name]; ok {
			return fmt.Errorf("slug %q defined twice (as %s and slug)", name, prev)
		}
		seen[name] = "slug"
	}

	for name, f := range c.Filtered {
		if err := validateName(name, min, max); err != nil {
			return err
		}
		if f.NS == "" {
			return fmt.Errorf("filtered slug %q has empty ns", name)
		}
		if f.Grep == "" {
			return fmt.Errorf("filtered slug %q has empty grep", name)
		}
		if prev, ok := seen[name]; ok {
			return fmt.Errorf("slug %q defined twice (as %s and filtered)", name, prev)
		}
		seen[name] = "filtered"
	}

	return nil
}

func validateName(name string, min, max int) error {
	n := len(name)
	if n < min {
		return fmt.Errorf("slug %q is %d chars; minimum is %d (set allowShorter: true to override)", name, n, min)
	}
	if max > 0 && n > max {
		return fmt.Errorf("slug %q is %d chars; maximum is %d (set allowLonger: true to override)", name, n, max)
	}
	if !slugNameRe.MatchString(name) {
		return fmt.Errorf("slug %q must match %s (lowercase alphanumeric, leading letter)", name, slugNameRe.String())
	}
	return nil
}

// ResolveSlug returns the namespace for a slug name, and whether the
// name is a plain slug (vs filtered).
func (c *Config) ResolveSlug(name string) (ns string, filtered *FilteredSlug, ok bool) {
	if f, exists := c.Filtered[name]; exists {
		return f.NS, &f, true
	}
	if ns, exists := c.Slugs[name]; exists {
		return ns, nil, true
	}
	return "", nil, false
}

// IsAll reports whether name is the all-namespaces slug.
func (c *Config) IsAll(name string) bool {
	return name == c.AllSlug
}

// Names returns all slug names (slugs + filtered), sorted.
func (c *Config) Names() []string {
	out := make([]string, 0, len(c.Slugs)+len(c.Filtered))
	for n := range c.Slugs {
		out = append(out, n)
	}
	for n := range c.Filtered {
		out = append(out, n)
	}
	// stable, deterministic order
	sortStrings(out)
	return out
}

// sortStrings is a tiny dependency-free sort to keep the package lean.
func sortStrings(s []string) {
	// insertion sort — N is tiny (dozens of slugs at most)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// String returns a human-readable summary for diagnostics.
func (c *Config) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d slugs, %d filtered, all=%q\n", len(c.Slugs), len(c.Filtered), c.AllSlug))
	return b.String()
}

// ExampleConfig returns a starter Config with a few common slugs.
// Users edit this to match their own namespaces.
func ExampleConfig() *Config {
	return &Config{
		Slugs: map[string]string{
			"clo":  "cloudflare",
			"sys":  "kube-system",
			"def":  "default",
			"argo": "argocd",
			"cert": "cert-manager",
			"roo":  "rook-ceph",
		},
		Filtered: map[string]FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
			"env": {NS: "kube-system", Grep: "envoy"},
		},
		AllSlug: "all",
	}
}

// WriteExample writes a starter config to path. Refuses to overwrite an
// existing file unless force is true.
func WriteExample(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config %s already exists (use --force to overwrite)", path)
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(ExampleConfig())
	if err != nil {
		return fmt.Errorf("marshal example config: %w", err)
	}
	header := []byte("# KubeXPander slugs config — edit to match your namespaces.\n" +
		"# Slug names must be 3-8 chars (lowercase alphanumeric, leading letter).\n" +
		"# Set allowShorter/allowLonger: true to override the length checks.\n" +
		"#\n" +
		"# Terminology: a 'slug' is a short namespace alias. `kclo` dispatches to\n" +
		"# the `cloudflare` namespace — `k` is the prefix, `clo` is the slug.\n\n")
	if err := os.WriteFile(path, append(header, data...), 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	fmt.Printf("wrote starter config to %s\n", path)
	fmt.Printf("edit it, then run: kxp install >> ~/.bashrc\n")
	return nil
}
