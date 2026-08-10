package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runDispatch executes the dispatch command in dry-run mode and returns the
// printed kubectl command.
func runDispatch(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cfgPath = filepath.Join("..", "..", "examples", "slugs.yaml")
	dryRun = true
	t.Cleanup(func() { cfgPath = ""; dryRun = false })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Errorf("close read end: %v", err)
		}
	}()

	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	cmd := dispatchCmd()
	cmd.SetArgs(args)
	cmd.SetOut(w)
	cmd.SetErr(w)
	runErr := cmd.Execute()

	if err := w.Close(); err != nil {
		t.Fatalf("close write end: %v", err)
	}
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String(), runErr
}

// kubectl flags after the slug name must reach kubectl instead of being
// parsed as sk flags (regression: `kclo delete pod foo --force` failed with
// "unknown flag: --force").
func TestDispatchPassesKubectlFlagsThrough(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "force delete",
			args: []string{"clo", "delete", "pod", "foo", "--force"},
			want: "kubectl -n cloudflare delete pod foo --force",
		},
		{
			name: "force delete with grace period",
			args: []string{"clo", "delete", "pod", "foo", "--force", "--grace-period=0"},
			want: "kubectl -n cloudflare delete pod foo --force --grace-period=0",
		},
		{
			name: "short flag",
			args: []string{"clo", "logs", "foo", "-f"},
			want: "kubectl -n cloudflare logs foo -f",
		},
		{
			name: "flag on filtered slug",
			args: []string{"cil", "delete", "pod", "foo", "--force"},
			want: "kubectl -n kube-system delete pod foo --force",
		},
		{
			name: "flag on all-namespaces slug",
			args: []string{"all", "get", "pods", "--show-labels"},
			want: "kubectl get pods --show-labels",
		},
		{
			name: "flag on cluster getter",
			args: []string{"gn", "describe", "node", "foo", "--show-events=false"},
			want: "kubectl describe node foo --show-events=false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runDispatch(t, tc.args...)
			if err != nil {
				t.Fatalf("dispatch %v: %v (output %q)", tc.args, err, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("dispatch %v = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// The dispatcher's own verbs still resolve after the parsing change.
func TestDispatchVerbsStillResolve(t *testing.T) {
	out, err := runDispatch(t, "clo", "o")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if want := "kubectl get pods -n cloudflare -o wide"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}
