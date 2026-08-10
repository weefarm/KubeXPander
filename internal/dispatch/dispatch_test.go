package dispatch

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/weefarm/38specialK/internal/config"
)

// captureStdout temporarily redirects os.Stdout to capture DryRun output.
// Returns a function to call to get the captured output and restore stdout.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w
	return func() string {
		w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		io.Copy(&buf, r)
		r.Close()
		return buf.String()
	}
}

// testConfig returns a Config with a few slugs for testing dispatch.
func testConfig() *config.Config {
	return &config.Config{
		Slugs: map[string]string{
			"clo": "cloudflare",
			"sys": "kube-system",
		},
		Filtered: map[string]config.FilteredSlug{
			"cil": {NS: "kube-system", Grep: "cilium"},
		},
		AllSlug: "all",
	}
}

func TestDispatchPlainSlugNoArgs(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", nil, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods -n cloudflare\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchPlainSlugO(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"o"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods -n cloudflare -o wide\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchPlainSlugOYaml(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"o", "yaml"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods -n cloudflare -o yaml\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchPlainSlugPassthrough(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"get", "deploy"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl -n cloudflare get deploy\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchFilteredSlugNoArgs(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "cil", nil, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods -n kube-system | grep cilium\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchFilteredSlugO(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "cil", []string{"o"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods -n kube-system -o wide | grep cilium\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchFilteredSlugPassthrough(t *testing.T) {
	// Pass-through should NOT be filtered — kubectl -n <ns> <args...>
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "cil", []string{"get", "deploy"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl -n kube-system get deploy\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchRfSingleName(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"rmf", "deployment", "foo"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := `kubectl patch deployment foo -n cloudflare -p {"metadata":{"finalizers":null}} --type merge` + "\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchRfMultipleNames(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"rmf", "deployment", "foo", "bar", "baz"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// Should produce one patch command per name
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 patch commands, got %d: %q", len(lines), out)
	}
	for _, line := range lines {
		if !strings.Contains(line, "kubectl patch deployment") {
			t.Errorf("expected patch command, got %q", line)
		}
		if !strings.Contains(line, "-n cloudflare") {
			t.Errorf("expected -n cloudflare in %q", line)
		}
	}
}

func TestDispatchRfTypeNameForm(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"rmf", "deployment/foo"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := `kubectl patch deployment/foo -n cloudflare -p {"metadata":{"finalizers":null}} --type merge` + "\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchRfNoArgs(t *testing.T) {
	cfg := testConfig()
	err := Dispatch(cfg, "clo", []string{"rmf"}, Options{DryRun: true})
	if err == nil {
		t.Fatal("expected error for rmf with no args, got nil")
	}
	if !strings.Contains(err.Error(), "missing arguments") {
		t.Errorf("expected 'missing arguments' error, got %q", err.Error())
	}
}

func TestDispatchRfOnlyType(t *testing.T) {
	cfg := testConfig()
	err := Dispatch(cfg, "clo", []string{"rmf", "deployment"}, Options{DryRun: true})
	if err == nil {
		t.Fatal("expected error for rmf with only type, got nil")
	}
	if !strings.Contains(err.Error(), "need at least") {
		t.Errorf("expected 'need at least' error, got %q", err.Error())
	}
}

func TestDispatchAllNoArgs(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "all", nil, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods --all-namespaces\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchAllO(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "all", []string{"o"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods --all-namespaces -o wide\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchAllOYaml(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "all", []string{"o", "yaml"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods --all-namespaces -o yaml\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchAllPassthrough(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "all", []string{"get", "ns"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get ns\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchAllRfRejected(t *testing.T) {
	cfg := testConfig()
	err := Dispatch(cfg, "all", []string{"rmf", "deployment", "foo"}, Options{DryRun: true})
	if err == nil {
		t.Fatal("expected error for rmf on all-namespaces slug, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("expected 'unsafe' error, got %q", err.Error())
	}
}

func TestDispatchAllGf(t *testing.T) {
	// lsf on all-namespaces should produce multiple kubectl get commands
	// (one per resource type). We just verify it produces output without error.
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "all", []string{"lsf"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// Should contain kubectl get commands for each type
	if !strings.Contains(out, "kubectl get pods") {
		t.Errorf("expected 'kubectl get pods' in output, got %q", out)
	}
	if !strings.Contains(out, "--all-namespaces") {
		t.Errorf("expected '--all-namespaces' in output, got %q", out)
	}
	// Should also scan cluster-scoped types
	if !strings.Contains(out, "kubectl get namespaces") {
		t.Errorf("expected 'kubectl get namespaces' in output, got %q", out)
	}
	if !strings.Contains(out, "kubectl get persistentvolumes") {
		t.Errorf("expected 'kubectl get persistentvolumes' in output, got %q", out)
	}
}

func TestDispatchGfNamespace(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"lsf"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// Should produce kubectl get commands for each namespaced type
	if !strings.Contains(out, "kubectl get pods -n cloudflare") {
		t.Errorf("expected 'kubectl get pods -n cloudflare' in output, got %q", out)
	}
	if !strings.Contains(out, "kubectl get deployments -n cloudflare") {
		t.Errorf("expected 'kubectl get deployments -n cloudflare' in output, got %q", out)
	}
	if !strings.Contains(out, "kubectl get ingress -n cloudflare") {
		t.Errorf("expected 'kubectl get ingress -n cloudflare' in output, got %q", out)
	}
	// Should NOT contain cluster-scoped types in a namespace scan
	if strings.Contains(out, "kubectl get namespaces") {
		t.Errorf("did not expect 'kubectl get namespaces' in namespace scan, got %q", out)
	}
}

func TestDispatchUnknownSlug(t *testing.T) {
	cfg := testConfig()
	err := Dispatch(cfg, "nonexistent", nil, Options{DryRun: true})
	if err == nil {
		t.Fatal("expected error for unknown slug, got nil")
	}
	if !strings.Contains(err.Error(), "no slug named") {
		t.Errorf("expected 'no slug named' error, got %q", err.Error())
	}
	// Error should list available slugs
	if !strings.Contains(err.Error(), "clo") {
		t.Errorf("expected error to list available slugs including 'clo', got %q", err.Error())
	}
}

func TestDispatchFilteredRfNotFiltered(t *testing.T) {
	// rmf on a filtered slug should NOT pipe through grep
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "cil", []string{"rmf", "deployment", "foo"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// Should be a plain patch command, NOT piped through grep
	if strings.Contains(out, "grep") {
		t.Errorf("rmf on filtered slug should NOT be piped through grep, got %q", out)
	}
	if !strings.Contains(out, "kubectl patch deployment foo -n kube-system") {
		t.Errorf("expected patch command in kube-system, got %q", out)
	}
}

func TestDispatchRfRejectsGlobbedFiles(t *testing.T) {
	cfg := testConfig()
	// Create a temp directory with files that look like shell-globbed arguments.
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.WriteFile("slugs.sh", []byte("test"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := os.WriteFile("slugs.yaml", []byte("test"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	err := Dispatch(cfg, "clo", []string{"rmf", "slugs.sh", "slugs.yaml"}, Options{DryRun: true})
	if err == nil {
		t.Fatal("expected error for rmf with globbed filenames, got nil")
	}
	if !strings.Contains(err.Error(), "local file") {
		t.Errorf("expected 'local file' error, got %q", err.Error())
	}
}

func TestDispatchRfAll(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"rmf", "--all"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// In dry-run we expect one scan command per resource type.
	for _, rtype := range finTypesNS {
		expected := "kubectl get " + rtype + " -n cloudflare"
		if !strings.Contains(out, expected) {
			t.Errorf("expected scan command for %s, got %q", rtype, out)
		}
	}
}

func TestDispatchRfLiteralStar(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "clo", []string{"rmf", "*"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if !strings.Contains(out, "kubectl get pods -n cloudflare") {
		t.Errorf("expected scan for pods, got %q", out)
	}
}

func TestConfirmRmfDryRunSkipsPrompt(t *testing.T) {
	// In DryRun mode, confirmRmf should return true without prompting.
	got := confirmRmf("deployment foo", "cloudflare", Options{DryRun: true})
	if !got {
		t.Error("confirmRmf should return true in DryRun mode")
	}
}

func TestConfirmRmfYes(t *testing.T) {
	// Simulate stdin with "y\n"
	old := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("y\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = old }()

	got := confirmRmf("deployment foo", "cloudflare", Options{DryRun: false})
	if !got {
		t.Error("confirmRmf should return true for 'y'")
	}
}

func TestConfirmRmfNo(t *testing.T) {
	// Simulate stdin with "n\n"
	old := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("n\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = old }()

	got := confirmRmf("deployment foo", "cloudflare", Options{DryRun: false})
	if got {
		t.Error("confirmRmf should return false for 'n'")
	}
}

func TestConfirmRmfEmptyCancel(t *testing.T) {
	// Simulate stdin with just "\n" (empty response)
	old := os.Stdin
	r, w, _ := os.Pipe()
	w.WriteString("\n")
	w.Close()
	os.Stdin = r
	defer func() { os.Stdin = old }()

	got := confirmRmf("deployment foo", "cloudflare", Options{DryRun: false})
	if got {
		t.Error("confirmRmf should return false for empty input")
	}
}

// --- Built-in cluster-scoped getters (kgn, kns) ---

func TestDispatchKgnNoArgs(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "gn", nil, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get nodes\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchKgnO(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "gn", []string{"o"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get nodes -o wide\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchKgnOYaml(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "gn", []string{"o", "yaml"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get nodes -o yaml\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchKgnPassthrough(t *testing.T) {
	// Pass-through should go straight to kubectl (no `get nodes` prefix).
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "gn", []string{"describe", "node", "foo"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl describe node foo\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchKnsNoArgs(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "ns", nil, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get namespaces\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchKnsO(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "ns", []string{"o"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get namespaces -o wide\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchKnsPassthrough(t *testing.T) {
	cfg := testConfig()
	restore := captureStdout(t)
	err := Dispatch(cfg, "ns", []string{"label", "ns", "foo", "key=value"}, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl label ns foo key=value\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestDispatchUserSlugOverridesBuiltin(t *testing.T) {
	// If a user has a slug named "gn", it should take precedence over the
	// built-in cluster getter.
	cfg := &config.Config{
		Slugs:   map[string]string{"gn": "kube-system"},
		AllSlug: "all",
	}
	restore := captureStdout(t)
	err := Dispatch(cfg, "gn", nil, Options{DryRun: true})
	out := restore()
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	expected := "kubectl get pods -n kube-system\n"
	if out != expected {
		t.Errorf("user slug should override built-in: expected %q, got %q", expected, out)
	}
}
