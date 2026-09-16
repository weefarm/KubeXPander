package config

import (
	"os"
	"path/filepath"
	"testing"
)

// helper: write a YAML config to a temp file and return its path
func writeTempConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeTempConfig(t, `
slugs:
  clo: cloudflare
  sys: kube-system
filtered:
  cil:
    ns: kube-system
    grep: cilium
allSlug: all
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Slugs["clo"] != "cloudflare" {
		t.Errorf("expected clo -> cloudflare, got %q", cfg.Slugs["clo"])
	}
	if cfg.Slugs["sys"] != "kube-system" {
		t.Errorf("expected sys -> kube-system, got %q", cfg.Slugs["sys"])
	}
	if cfg.Filtered["cil"].NS != "kube-system" {
		t.Errorf("expected cil.NS = kube-system, got %q", cfg.Filtered["cil"].NS)
	}
	if cfg.Filtered["cil"].Grep != "cilium" {
		t.Errorf("expected cil.Grep = cilium, got %q", cfg.Filtered["cil"].Grep)
	}
	if cfg.AllSlug != "all" {
		t.Errorf("expected AllSlug = all, got %q", cfg.AllSlug)
	}
}

func TestLoadDefaultAllSlug(t *testing.T) {
	// allSlug not specified — should default to "all"
	path := writeTempConfig(t, `
slugs:
  clo: cloudflare
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.AllSlug != "all" {
		t.Errorf("expected default AllSlug = all, got %q", cfg.AllSlug)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/slugs.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTempConfig(t, `
slugs:
  clo: cloudflare
  this is: not: valid yaml
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestValidateNameTooShort(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"ab": "cloudflare", // 2 chars, below min of 3
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for 2-char slug name, got nil")
	}
}

func TestValidateNameTooShortWithAllowShorter(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"ab": "cloudflare",
		},
		AllowShorter: true,
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error with allowShorter, got: %v", err)
	}
}

func TestValidateNameTooLong(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"toolongname": "cloudflare", // 11 chars, above max of 8
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for 11-char slug name, got nil")
	}
}

func TestValidateNameTooLongWithAllowLonger(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"toolongname": "cloudflare",
		},
		AllowLonger: true,
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error with allowLonger, got: %v", err)
	}
}

func TestValidateNameBoundaryMin(t *testing.T) {
	// 3 chars — exactly at min, should pass
	cfg := &Config{
		Slugs: map[string]string{
			"clo": "cloudflare",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected 3-char name to pass, got: %v", err)
	}
}

func TestValidateNameBoundaryMax(t *testing.T) {
	// 8 chars — exactly at max, should pass
	cfg := &Config{
		Slugs: map[string]string{
			"eightch": "cloudflare", // 7 chars
			"eightchr": "cloudflare", // 8 chars
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected 8-char name to pass, got: %v", err)
	}
}

func TestValidateNameInvalidChars(t *testing.T) {
	tests := []string{
		"Clo",  // uppercase
		"clo-", // hyphen
		"clo_", // underscore
		"1clo", // leading digit
		"clo!", // special char
		" clo", // leading space
	}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{
				Slugs: map[string]string{
					name: "cloudflare",
				},
				AllowShorter: true, // bypass length check to isolate char check
			}
			err := cfg.Validate()
			if err == nil {
				t.Errorf("expected error for invalid slug name %q, got nil", name)
			}
		})
	}
}

func TestValidateEmptyNamespace(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"clo": "", // empty namespace
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty namespace, got nil")
	}
}

func TestValidateFilteredEmptyNS(t *testing.T) {
	cfg := &Config{
		Filtered: map[string]FilteredSlug{
			"cil": {NS: "", Grep: "cilium"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty filtered NS, got nil")
	}
}

func TestValidateFilteredEmptyGrep(t *testing.T) {
	cfg := &Config{
		Filtered: map[string]FilteredSlug{
			"cil": {NS: "kube-system", Grep: ""},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty filtered Grep, got nil")
	}
}

func TestValidateCollisionSlugAndFiltered(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"cil": "kube-system",
		},
		Filtered: map[string]FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for slug defined as both plain and filtered, got nil")
	}
}

func TestResolveSlugPlain(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"clo": "cloudflare",
		},
	}
	ns, filtered, ok := cfg.ResolveSlug("clo")
	if !ok {
		t.Fatal("expected ok=true for existing slug")
	}
	if ns != "cloudflare" {
		t.Errorf("expected ns=cloudflare, got %q", ns)
	}
	if filtered != nil {
		t.Errorf("expected filtered=nil for plain slug, got %+v", filtered)
	}
}

func TestResolveSlugFiltered(t *testing.T) {
	cfg := &Config{
		Filtered: map[string]FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
		},
	}
	ns, filtered, ok := cfg.ResolveSlug("cil")
	if !ok {
		t.Fatal("expected ok=true for existing filtered slug")
	}
	if ns != "kube-system" {
		t.Errorf("expected ns=kube-system, got %q", ns)
	}
	if filtered == nil {
		t.Fatal("expected filtered != nil for filtered slug")
	}
	if filtered.Grep != "cilium" {
		t.Errorf("expected grep=cilium, got %q", filtered.Grep)
	}
}

func TestResolveSlugNotFound(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{"clo": "cloudflare"},
	}
	_, _, ok := cfg.ResolveSlug("nonexistent")
	if ok {
		t.Fatal("expected ok=false for nonexistent slug")
	}
}

func TestIsAll(t *testing.T) {
	cfg := &Config{AllSlug: "all"}
	if !cfg.IsAll("all") {
		t.Error("expected IsAll(\"all\") = true")
	}
	if cfg.IsAll("clo") {
		t.Error("expected IsAll(\"clo\") = false")
	}
}

func TestIsAllCustomName(t *testing.T) {
	cfg := &Config{AllSlug: "cluster"}
	if !cfg.IsAll("cluster") {
		t.Error("expected IsAll(\"cluster\") = true with custom AllSlug")
	}
	if cfg.IsAll("all") {
		t.Error("expected IsAll(\"all\") = false with custom AllSlug=cluster")
	}
}

func TestNames(t *testing.T) {
	cfg := &Config{
		Slugs: map[string]string{
			"clo":  "cloudflare",
			"sys":  "kube-system",
			"argo": "argocd",
		},
		Filtered: map[string]FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
		},
	}
	names := cfg.Names()
	expected := []string{"argo", "cil", "clo", "sys"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d: %v", len(expected), len(names), names)
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestNamesEmpty(t *testing.T) {
	cfg := &Config{}
	names := cfg.Names()
	if len(names) != 0 {
		t.Errorf("expected empty names, got %v", names)
	}
}

func TestString(t *testing.T) {
	cfg := &Config{
		Slugs:    map[string]string{"clo": "cloudflare"},
		Filtered: map[string]FilteredSlug{"cil": {NS: "kube-system", Grep: "cilium"}},
		AllSlug:  "all",
	}
	s := cfg.String()
	// Should contain the counts and allSlug name
	if !contains(s, "1 slugs") {
		t.Errorf("expected String() to contain '1 slugs', got %q", s)
	}
	if !contains(s, "1 filtered") {
		t.Errorf("expected String() to contain '1 filtered', got %q", s)
	}
	if !contains(s, "all=\"all\"") {
		t.Errorf("expected String() to contain all=\"all\", got %q", s)
	}
}

func TestExampleConfig(t *testing.T) {
	cfg := ExampleConfig()
	if cfg.Slugs["clo"] != "cloudflare" {
		t.Errorf("expected example clo -> cloudflare, got %q", cfg.Slugs["clo"])
	}
	if cfg.AllSlug != "all" {
		t.Errorf("expected example AllSlug = all, got %q", cfg.AllSlug)
	}
	// Example config includes "rc" (2 chars) which needs allowShorter
	cfg.AllowShorter = true
	if err := cfg.Validate(); err != nil {
		t.Errorf("example config failed validation: %v", err)
	}
}

func TestWriteExample(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "slugs.yaml")

	// Should create parent dirs and write the file
	if err := WriteExample(path, false); err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	// File should exist and be loadable
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load of written example failed: %v", err)
	}
	if cfg.Slugs["clo"] != "cloudflare" {
		t.Errorf("expected clo -> cloudflare in written example, got %q", cfg.Slugs["clo"])
	}
}

func TestWriteExampleRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")

	// First write succeeds
	if err := WriteExample(path, false); err != nil {
		t.Fatalf("first WriteExample failed: %v", err)
	}

	// Second write without force should fail
	err := WriteExample(path, false)
	if err == nil {
		t.Fatal("expected error for overwrite without force, got nil")
	}
}

func TestWriteExampleForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")

	// First write
	if err := WriteExample(path, false); err != nil {
		t.Fatalf("first WriteExample failed: %v", err)
	}

	// Second write with force should succeed
	if err := WriteExample(path, true); err != nil {
		t.Fatalf("WriteExample with force failed: %v", err)
	}
}

func TestDefaultPath(t *testing.T) {
	// With XDG_CONFIG_HOME set
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath failed: %v", err)
	}
	if path != "/tmp/xdg-test/kxp/slugs.yaml" {
		t.Errorf("expected /tmp/xdg-test/kxp/slugs.yaml, got %q", path)
	}
}

func TestDefaultPathFallback(t *testing.T) {
	// Without XDG_CONFIG_HOME — should fall back to ~/.config/kxp/slugs.yaml
	t.Setenv("XDG_CONFIG_HOME", "")
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath failed: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "kxp", "slugs.yaml")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// TestWriteExampleFilePermissions verifies that the config file is created
// with mode 0o644 (user-readable and user-writable). This is a regression
// guard for the permission-denied issue reported in #5, where the config
// file ended up unreadable/unwritable by the user after install.
func TestWriteExampleFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")

	if err := WriteExample(path, false); err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0o644 {
		t.Errorf("expected config file mode 0o644, got 0o%o", mode)
	}
}

// TestWriteExampleDirPermissions verifies that the parent directory is
// created with mode 0o755 (user-readable, user-writable, user-executable).
// Without an executable+readable parent dir, the user can't access the file
// inside it even if the file itself has correct permissions.
func TestWriteExampleDirPermissions(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested", "config", "kxp")
	path := filepath.Join(subdir, "slugs.yaml")

	if err := WriteExample(path, false); err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0o755 {
		t.Errorf("expected config dir mode 0o755, got 0o%o", mode)
	}
}

// TestWriteExampleFileIsWritable verifies that the config file can be opened
// for writing after WriteExample creates it. This is the direct regression
// test for the "Permission denied" error from #5 — if the file can't be
// opened for writing by the same process that created it, the user can't
// edit it with $EDITOR either.
func TestWriteExampleFileIsWritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")

	if err := WriteExample(path, false); err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	// Should be able to open the file for writing (simulates $EDITOR)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("config file not writable after WriteExample: %v", err)
	}
	f.Close()
}

// TestWriteExampleFileIsReadable verifies that the config file can be read
// back after creation. Paired with the writability test, this confirms the
// file is accessible to the user — the core expectation broken in #5.
func TestWriteExampleFileIsReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")

	if err := WriteExample(path, false); err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	// Should be able to open the file for reading
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("config file not readable after WriteExample: %v", err)
	}
	f.Close()
}

// TestWriteExampleRoundTrip verifies the full init→load round-trip: the
// example config written by WriteExample can be loaded back by Load without
// errors, and the loaded config matches the example. This is the foundation
// of the install flow — init writes a valid config, and install reads it.
func TestWriteExampleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slugs.yaml")

	if err := WriteExample(path, false); err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load of WriteExample output failed: %v", err)
	}

	example := ExampleConfig()
	example.AllowShorter = true // WriteExample doesn't set AllowShorter; Load picks up the YAML

	if cfg.AllSlug != example.AllSlug {
		t.Errorf("AllSlug mismatch: got %q, want %q", cfg.AllSlug, example.AllSlug)
	}
	if len(cfg.Slugs) != len(example.Slugs) {
		t.Errorf("Slugs count mismatch: got %d, want %d", len(cfg.Slugs), len(example.Slugs))
	}
	for k, v := range example.Slugs {
		if cfg.Slugs[k] != v {
			t.Errorf("slug %q: got %q, want %q", k, cfg.Slugs[k], v)
		}
	}
}

// contains is a helper for substring checking (avoiding strings.Contains import
// in this test file — keeps it dependency-free for the config package tests).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
