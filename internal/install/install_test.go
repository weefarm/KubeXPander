package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weefarm/KubeXPander/internal/config"
)

func TestEmitBasicStructure(t *testing.T) {
	cfg := &config.Config{
		Slugs: map[string]string{
			"clo": "cloudflare",
			"sys": "kube-system",
		},
		AllSlug: "all",
	}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	// Should contain the header
	if !strings.Contains(out, "auto-generated") {
		t.Error("expected header with 'auto-generated'")
	}

	// Should contain the kxpd alias
	if !strings.Contains(out, `alias kxpd="kxp dispatch"`) {
		t.Error("expected kxpd alias in output")
	}

	// Should contain the all-namespaces function
	if !strings.Contains(out, "kall(){ kxp dispatch all \"$@\"; }") {
		t.Error("expected kall function in output")
	}

	// Should contain plain slug functions
	if !strings.Contains(out, "kclo(){ kxp dispatch clo \"$@\"; }") {
		t.Error("expected kclo function in output")
	}
	if !strings.Contains(out, "ksys(){ kxp dispatch sys \"$@\"; }") {
		t.Error("expected ksys function in output")
	}
}

func TestEmitFilteredSlugs(t *testing.T) {
	cfg := &config.Config{
		Filtered: map[string]config.FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
		},
		AllSlug: "all",
	}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "kcil(){ kxp dispatch cil \"$@\"; }") {
		t.Error("expected kcil function in output")
	}
}

func TestEmitGfGuard(t *testing.T) {
	cfg := &config.Config{AllSlug: "all"}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "lsf(){") {
		t.Error("expected lsf guard function in output")
	}
	if !strings.Contains(out, "Usage: kall lsf") {
		t.Error("expected lsf usage hint in output")
	}
	if !strings.Contains(out, "return 1") {
		t.Error("expected lsf guard to return 1")
	}
}

func TestEmitCompletion(t *testing.T) {
	cfg := &config.Config{
		Slugs:   map[string]string{"clo": "cloudflare"},
		AllSlug: "all",
	}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	// Should contain the completion function
	if !strings.Contains(out, "_kxp_dispatch_complete") {
		t.Error("expected completion function in output")
	}

	// Should register completion for each slug function + all
	if !strings.Contains(out, "complete -F _kxp_dispatch_complete kclo") {
		t.Error("expected completion registration for kclo")
	}
	if !strings.Contains(out, "complete -F _kxp_dispatch_complete kall") {
		t.Error("expected completion registration for kall")
	}
}

func TestEmitClusterGetBuiltins(t *testing.T) {
	cfg := &config.Config{AllSlug: "all"}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	// Should emit kgn and kns functions
	if !strings.Contains(out, "kgn(){ kxp dispatch gn \"$@\"; }") {
		t.Error("expected kgn function in output")
	}
	if !strings.Contains(out, "kns(){ kxp dispatch ns \"$@\"; }") {
		t.Error("expected kns function in output")
	}

	// Should register completions for kgn and kns
	if !strings.Contains(out, "complete -F _kxp_dispatch_complete kgn") {
		t.Error("expected completion registration for kgn")
	}
	if !strings.Contains(out, "complete -F _kxp_dispatch_complete kns") {
		t.Error("expected completion registration for kns")
	}
}

func TestEmitDeterministicOrder(t *testing.T) {
	// Emit should produce deterministic output regardless of map iteration order
	cfg := &config.Config{
		Slugs: map[string]string{
			"clo":  "cloudflare",
			"sys":  "kube-system",
			"argo": "argocd",
		},
		AllSlug: "all",
	}

	var buf1, buf2 strings.Builder
	if err := Emit(cfg, &buf1); err != nil {
		t.Fatalf("first Emit failed: %v", err)
	}
	if err := Emit(cfg, &buf2); err != nil {
		t.Fatalf("second Emit failed: %v", err)
	}

	if buf1.String() != buf2.String() {
		t.Error("expected deterministic output across calls")
	}
}

func TestEmitSortedOrder(t *testing.T) {
	cfg := &config.Config{
		Slugs: map[string]string{
			"sys":  "kube-system",
			"clo":  "cloudflare",
			"argo": "argocd",
		},
		AllSlug: "all",
	}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	// Functions should appear in sorted order: argo, clo, sys
	idxArgo := strings.Index(out, "kargo(){")
	idxClo := strings.Index(out, "kclo(){")
	idxSys := strings.Index(out, "ksys(){")

	if idxArgo < 0 || idxClo < 0 || idxSys < 0 {
		t.Fatalf("missing expected functions in output")
	}
	if !(idxArgo < idxClo && idxClo < idxSys) {
		t.Errorf("expected sorted order (argo < clo < sys), got indices: argo=%d clo=%d sys=%d",
			idxArgo, idxClo, idxSys)
	}
}

func TestEmitCustomAllSlug(t *testing.T) {
	cfg := &config.Config{
		Slugs:    map[string]string{"clo": "cloudflare"},
		AllSlug:  "cluster",
	}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "kcluster(){ kxp dispatch cluster \"$@\"; }") {
		t.Error("expected kcluster function with custom AllSlug")
	}
}

func TestEmitEmptyConfig(t *testing.T) {
	cfg := &config.Config{AllSlug: "all"}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	// Should still have header, kxpd alias, kall, lsf guard, and completion
	if !strings.Contains(out, "auto-generated") {
		t.Error("expected header even with empty config")
	}
	if !strings.Contains(out, `alias kxpd="kxp dispatch"`) {
		t.Error("expected kxpd alias even with empty config")
	}
	if !strings.Contains(out, "kall(){ kxp dispatch all \"$@\"; }") {
		t.Error("expected kall function even with empty config")
	}
}

func TestEmitRfCompletionTypes(t *testing.T) {
	// The completion block should include resource types for rmf completion
	cfg := &config.Config{AllSlug: "all"}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	out := buf.String()

	expectedTypes := []string{"pods", "deployments", "services", "ingress", "configmaps", "secrets"}
	for _, typ := range expectedTypes {
		if !strings.Contains(out, typ) {
			t.Errorf("expected completion to include resource type %q", typ)
		}
	}
}

// TestEmitValidBashSyntax runs `bash -n` on the emitted shell snippet to
// verify it's syntactically valid bash. If the generated functions have a
// syntax error, sourcing the snippet from ~/.bashrc would break the user's
// shell — this test catches that before it ships.
func TestEmitValidBashSyntax(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available in test environment")
	}

	cfg := &config.Config{
		Slugs: map[string]string{
			"clo":  "cloudflare",
			"sys":  "kube-system",
			"argo": "argocd",
		},
		Filtered: map[string]config.FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
		},
		AllSlug: "all",
	}

	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Write to a temp file and syntax-check with `bash -n`
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "slugs.sh")
	if err := os.WriteFile(scriptPath, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write temp script: %v", err)
	}

	cmd := exec.Command("bash", "-n", scriptPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("emitted shell snippet has bash syntax errors: %v\noutput:\n%s", err, output)
	}
}

// TestEmitValidBashSyntaxEmptyConfig verifies the snippet is valid bash even
// with an empty config (no slugs, just the header, kxpd alias, kall, lsf
// guard, and completion block).
func TestEmitValidBashSyntaxEmptyConfig(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available in test environment")
	}

	cfg := &config.Config{AllSlug: "all"}
	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "slugs.sh")
	if err := os.WriteFile(scriptPath, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("write temp script: %v", err)
	}

	cmd := exec.Command("bash", "-n", scriptPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("emitted shell snippet (empty config) has bash syntax errors: %v\noutput:\n%s", err, output)
	}
}

// TestInitInstallFlow is an end-to-end test of the full install process:
// WriteExample (kxp init) → Load → Emit (kxp install) → bash -n. This is the
// exact sequence a user follows, and it verifies that the config written by
// init is loadable by install, and that the resulting shell snippet is valid
// bash. Regression guard for the install flow issues reported in #5.
func TestInitInstallFlow(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available in test environment")
	}

	// Step 1: kxp init — write a starter config to a temp dir
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".config", "kxp", "slugs.yaml")
	if err := config.WriteExample(cfgPath, false); err != nil {
		t.Fatalf("WriteExample (kxp init) failed: %v", err)
	}

	// Step 2: verify the config file has correct permissions (regression
	// guard for the permission-denied issue from #5)
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Errorf("config file mode: expected 0o644, got 0o%o", mode)
	}

	// Step 3: kxp install — load the config and emit the shell snippet
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load (kxp install config read) failed: %v", err)
	}

	var buf strings.Builder
	if err := Emit(cfg, &buf); err != nil {
		t.Fatalf("Emit (kxp install) failed: %v", err)
	}
	out := buf.String()

	// Step 4: verify the emitted snippet is valid bash
	scriptPath := filepath.Join(dir, "slugs.sh")
	if err := os.WriteFile(scriptPath, []byte(out), 0o644); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	cmd := exec.Command("bash", "-n", scriptPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("emitted shell snippet has bash syntax errors: %v\noutput:\n%s", err, output)
	}

	// Step 5: verify the snippet contains functions for the example slugs
	// (confirms the config round-tripped correctly through the full flow)
	expectedFunctions := []string{
		"kall(){ kxp dispatch all \"$@\"; }",
		"kclo(){ kxp dispatch clo \"$@\"; }",
		"ksys(){ kxp dispatch sys \"$@\"; }",
		`alias kxpd="kxp dispatch"`,
	}
	for _, fn := range expectedFunctions {
		if !strings.Contains(out, fn) {
			t.Errorf("expected emitted snippet to contain %q", fn)
		}
	}
}

// TestInitInstallFlowIdempotent verifies that re-running the init→install
// flow produces the same output. The convenience install script from #5
// must be safe to re-run, and this test confirms the underlying flow is
// deterministic across invocations.
func TestInitInstallFlowIdempotent(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available in test environment")
	}

	emit := func() string {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "slugs.yaml")
		if err := config.WriteExample(cfgPath, false); err != nil {
			t.Fatalf("WriteExample failed: %v", err)
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		var buf strings.Builder
		if err := Emit(cfg, &buf); err != nil {
			t.Fatalf("Emit failed: %v", err)
		}
		return buf.String()
	}

	first := emit()
	second := emit()

	if first != second {
		t.Error("expected identical output across two init→install runs")
	}
}
