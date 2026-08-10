// Package dispatch implements the sk verb dispatcher.
//
// The dispatch contract mirrors the bash v0 reference:
//
//	sk dispatch <slug>               -> kubectl get pods -n <ns>
//	sk dispatch <slug> o             -> kubectl get pods -n <ns> -o wide
//	sk dispatch <slug> o yaml        -> kubectl get pods -n <ns> -o yaml
//	sk dispatch <slug> lsf            -> list resources with finalizers in <ns>
//	sk dispatch <slug> rmf <type> <name> [name...]  -> strip finalizers
//	sk dispatch <slug> rmf <type/name>              -> strip finalizers (single-arg form)
//	sk dispatch <slug> <anything else>             -> kubectl -n <ns> <anything else>
//
// For filtered slugs (kcil/kenv style), the default listing and its -o
// variant are piped through grep; everything else passes through unfiltered.
//
// The all-namespaces slug (default "all") supports `o` and `lsf` but NOT
// `rmf` — patching across namespaces is too easy to misfire.
//
// Users typically invoke the generated shell functions (kclo, ksys, ...),
// but may also use `skd <slug>` if they install the optional `skd` alias
// (`alias skd="sk dispatch"`), which the install command emits.
package dispatch

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/weefarm/38specialK/internal/config"
)

// finTypesNS is the curated list of namespaced resource types scanned by lsf.
// Kept short for speed and predictability; edit here to add/remove types.
//
// ingress is included deliberately: on clusters using Cilium Gateway API
// in place of a conventional ingress controller, scanning for finalizers on
// ingress objects catches cases where an AI agent (or a distracted human)
// has added an ingress-type object that the Gateway API controller doesn't
// reconcile, leaving it stuck terminating.
var finTypesNS = []string{
	"pods",
	"deployments",
	"statefulsets",
	"daemonsets",
	"replicasets",
	"persistentvolumeclaims",
	"services",
	"ingress",
	"configmaps",
	"secrets",
}

// finTypesCluster is the curated list of cluster-scoped types scanned by lsf.
var finTypesCluster = []string{
	"namespaces",
	"persistentvolumes",
}

// clusterGetBuiltins maps built-in cluster-scoped dispatch names to the
// kubectl resource they list. These are convenience shortcuts (kgn, kns)
// that bypass the namespace-slug model — they have no namespace.
//
// User slugs always take precedence over built-ins: Dispatch only consults
// this table when the name is NOT a configured slug. This keeps user configs
// authoritative and avoids surprising behavior if a user redefines `gn` or
// `ns` to mean something else.
var clusterGetBuiltins = map[string]string{
	"gn": "nodes",
	"ns": "namespaces",
}

// Options controls dispatch behavior.
type Options struct {
	// DryRun prints the kubectl command that would run instead of executing it.
	DryRun bool
}

// Dispatch resolves name against cfg and executes the appropriate kubectl
// command with the given args.
//
// Resolution order: all-namespaces slug → user slugs/filtered → built-in
// cluster-scoped getters (gn, ns). User slugs win over built-ins so configs
// remain authoritative.
func Dispatch(cfg *config.Config, name string, args []string, opts Options) error {
	if cfg.IsAll(name) {
		return dispatchAll(args, opts)
	}

	if ns, filtered, ok := cfg.ResolveSlug(name); ok {
		return dispatchNS(name, ns, filtered, args, opts)
	}

	if resource, isBuiltin := clusterGetBuiltins[name]; isBuiltin {
		return dispatchClusterGet(resource, args, opts)
	}

	return fmt.Errorf("no slug named %q in config (have: %s)", name, strings.Join(cfg.Names(), ", "))
}

// dispatchNS handles a namespace-scoped slug (plain or filtered).
func dispatchNS(name, ns string, filtered *config.FilteredSlug, args []string, opts Options) error {
	// No args -> default pod listing (filtered if applicable).
	if len(args) == 0 {
		return runFilteredPipe(filtered, []string{"get", "pods", "-n", ns}, opts)
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "o":
		// -o wide (or custom -o format if more args follow).
		if len(rest) == 0 {
			return runFilteredPipe(filtered, []string{"get", "pods", "-n", ns, "-o", "wide"}, opts)
		}
		return runFilteredPipe(filtered, append([]string{"get", "pods", "-n", ns, "-o"}, rest...), opts)

	case "lsf":
		// List resources with finalizers in this namespace.
		return listFinalizersNS(ns, opts)

	case "rmf":
		// Strip finalizers. NOT filtered even for filtered slugs —
		// patching is a deliberate act and grep would obscure the target.
		return stripFinalizersNS(name, ns, rest, opts)

	default:
		// Pass-through: kubectl -n <ns> <args...>
		return runKubectl(append([]string{"-n", ns}, args...), opts)
	}
}

// dispatchAll handles the all-namespaces slug.
// `rmf` is intentionally unsupported here — patching across namespaces is
// too easy to misfire. Use a namespace-scoped slug instead.
func dispatchAll(args []string, opts Options) error {
	if len(args) == 0 {
		return runKubectl([]string{"get", "pods", "--all-namespaces"}, opts)
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "o":
		if len(rest) == 0 {
			return runKubectl([]string{"get", "pods", "--all-namespaces", "-o", "wide"}, opts)
		}
		return runKubectl(append([]string{"get", "pods", "--all-namespaces", "-o"}, rest...), opts)

	case "lsf":
		return listFinalizersAll(opts)

	case "rmf":
		fmt.Fprintln(os.Stderr, "rmf is intentionally not supported on the all-namespaces slug.")
		fmt.Fprintln(os.Stderr, "Use a namespace-scoped slug instead (e.g. sk dispatch clo rmf deployment foo).")
		return fmt.Errorf("rmf on all-namespaces is unsafe")

	default:
		return runKubectl(args, opts)
	}
}

// dispatchClusterGet handles built-in cluster-scoped getters (kgn, kns).
//
//	sk dispatch gn             -> kubectl get nodes
//	sk dispatch gn o           -> kubectl get nodes -o wide
//	sk dispatch gn o yaml      -> kubectl get nodes -o yaml
//	sk dispatch gn label node foo key=value  -> kubectl label node foo key=value (pass-through)
//
// The `o` verb is handled like namespace slugs; everything else is passed
// straight to kubectl without a `get <resource>` prefix, so non-get commands
// (describe, label, edit, ...) work naturally.
func dispatchClusterGet(resource string, args []string, opts Options) error {
	if len(args) == 0 {
		return runKubectl([]string{"get", resource}, opts)
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "o":
		if len(rest) == 0 {
			return runKubectl([]string{"get", resource, "-o", "wide"}, opts)
		}
		return runKubectl(append([]string{"get", resource, "-o"}, rest...), opts)
	default:
		return runKubectl(args, opts)
	}
}

// listFinalizersNS scans a single namespace for resources with finalizers set.
func listFinalizersNS(ns string, opts Options) error {
	jsonpath := `{range .items[?(@.metadata.finalizers)]}{.kind}{"/"}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}`
	for _, t := range finTypesNS {
		args := []string{"get", t, "-n", ns, "-o", jsonpath}
		if err := runKubectlSilent(args, opts); err != nil {
			// Skip types that don't exist in the namespace rather than failing.
			continue
		}
	}
	return nil
}

// listFinalizersAll scans the whole cluster (all namespaces + cluster-scoped).
func listFinalizersAll(opts Options) error {
	nsJsonpath := `{range .items[?(@.metadata.finalizers)]}{.metadata.namespace}{"/"}{.kind}{"/"}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}`
	for _, t := range finTypesNS {
		args := []string{"get", t, "--all-namespaces", "-o", nsJsonpath}
		_ = runKubectlSilent(args, opts)
	}
	clusterJsonpath := `{range .items[?(@.metadata.finalizers)]}{.kind}{"/"}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}`
	for _, t := range finTypesCluster {
		args := []string{"get", t, "-o", clusterJsonpath}
		_ = runKubectlSilent(args, opts)
	}
	return nil
}

// confirmRmf prompts the user for confirmation before stripping finalizers.
// Returns true if the user typed "y" or "yes", false otherwise.
// Skipped in DryRun mode (tests and preview).
func confirmRmf(target, ns string, opts Options) bool {
	if opts.DryRun {
		return true
	}
	fmt.Printf("Are you sure you want to remove finalizers from %s in namespace %s? [y/N] ", target, ns)
	reader := bufio.NewReader(os.Stdin)
	resp, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	resp = strings.TrimSpace(strings.ToLower(resp))
	return resp == "y" || resp == "yes"
}

// stripFinalizersNS patches finalizers to null on one or more resources.
//
//	sk dispatch clo rmf --all                    -> patch all resources with finalizers
//	sk dispatch clo rmf deployment foo           -> patch deployment/foo
//	sk dispatch clo rmf deployment foo bar baz  -> patch each name
//	sk dispatch clo rmf deployment/foo          -> single type/name form
//
// A confirmation prompt gates the actual dispatch — the user must type "y" or
// "yes" to proceed; anything else cancels. The prompt is skipped in DryRun mode.
func stripFinalizersNS(caller, ns string, args []string, opts Options) error {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s rmf --all | %s rmf <type> <name> [name...] | %s rmf <type/name>\n", caller, caller, caller)
		return fmt.Errorf("rmf: missing arguments")
	}

	// --all or a literal wildcard means every resource with finalizers in the namespace.
	if len(args) == 1 && (args[0] == "--all" || args[0] == "*") {
		return stripFinalizersAllNS(ns, opts)
	}

	// Reject arguments that are local files; this usually means the user ran
	// `rmf *` and the shell expanded the star into filenames.
	if err := validateRmfArgs(args); err != nil {
		return err
	}

	// type/name single-arg form.
	if len(args) == 1 && strings.Contains(args[0], "/") {
		if !confirmRmf(args[0], ns, opts) {
			fmt.Fprintln(os.Stderr, "rmf: cancelled.")
			return fmt.Errorf("rmf: cancelled by user")
		}
		patch := []byte(`{"metadata":{"finalizers":null}}`)
		return runKubectlPatch(args[0], ns, patch, opts)
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s rmf <type> <name> [name...] | %s rmf <type/name>\n", caller, caller)
		return fmt.Errorf("rmf: need at least <type> and <name>")
	}

	rtype := args[0]
	names := args[1:]
	if !confirmRmf(rtype+" "+strings.Join(names, " "), ns, opts) {
		fmt.Fprintln(os.Stderr, "rmf: cancelled.")
		return fmt.Errorf("rmf: cancelled by user")
	}
	patch := []byte(`{"metadata":{"finalizers":null}}`)
	for _, name := range names {
		if err := runKubectlPatchType(rtype, name, ns, patch, opts); err != nil {
			return err
		}
	}
	return nil
}

// validateRmfArgs rejects arguments that look like shell-globbed filenames.
// If the user runs `rmf *` in a directory with files, bash expands the star
// into filenames before sk sees the command; this catches that mistake.
func validateRmfArgs(args []string) error {
	for _, a := range args {
		if _, err := os.Stat(a); err == nil {
			return fmt.Errorf("rmf: argument %q looks like a local file; did you mean 'rmf --all' or forget to quote a wildcard?", a)
		}
	}
	return nil
}

// stripFinalizersAllNS patches finalizers to null on every resource in the
// namespace that currently has finalizers set. It scans the resource types in
// finTypesNS and prompts for confirmation once before dispatching patches.
func stripFinalizersAllNS(ns string, opts Options) error {
	if !confirmRmf("all resources with finalizers", ns, opts) {
		fmt.Fprintln(os.Stderr, "rmf: cancelled.")
		return fmt.Errorf("rmf: cancelled by user")
	}

	patch := []byte(`{"metadata":{"finalizers":null}}`)
	for _, rtype := range finTypesNS {
		names, err := finalizerResourceNames(rtype, ns, opts)
		if err != nil {
			// Resource type may not exist in this cluster; skip it.
			continue
		}
		for _, name := range names {
			if err := runKubectlPatchType(rtype, name, ns, patch, opts); err != nil {
				return err
			}
		}
	}
	return nil
}

// finalizerResourceNames returns the names of resources of the given type in
// the namespace that have at least one finalizer set. In DryRun mode it prints
// the kubectl command that would be used and returns an empty list, because
// the real cluster state is not available.
func finalizerResourceNames(rtype, ns string, opts Options) ([]string, error) {
	jsonpath := `{range .items[?(@.metadata.finalizers)]}{.metadata.name}{"\n"}{end}`
	args := []string{"get", rtype, "-n", ns, "-o", jsonpath}
	if opts.DryRun {
		fmt.Printf("kubectl %s\n", strings.Join(args, " "))
		return nil, nil
	}
	out, err := runKubectlOutput(args, opts)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// --- kubectl runners ---

// runKubectl executes kubectl with the given args, streaming stdout/stderr.
func runKubectl(args []string, opts Options) error {
	if opts.DryRun {
		fmt.Printf("kubectl %s\n", strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runKubectlOutput executes kubectl and captures stdout. Used for operations
// that need to inspect the returned data before dispatching further commands.
// In DryRun mode it prints the command and returns empty output.
func runKubectlOutput(args []string, opts Options) ([]byte, error) {
	if opts.DryRun {
		fmt.Printf("kubectl %s\n", strings.Join(args, " "))
		return nil, nil
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// runKubectlSilent runs kubectl with stdout to the terminal but stderr suppressed.
// Used by lsf scans so missing resource types don't clutter output.
func runKubectlSilent(args []string, opts Options) error {
	if opts.DryRun {
		fmt.Printf("kubectl %s\n", strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = nil
	return cmd.Run()
}

// runFilteredPipe runs a kubectl command, piping through grep if f is set.
func runFilteredPipe(f *config.FilteredSlug, kubectlArgs []string, opts Options) error {
	if opts.DryRun {
		if f != nil {
			fmt.Printf("kubectl %s | grep %s\n", strings.Join(kubectlArgs, " "), f.Grep)
		} else {
			fmt.Printf("kubectl %s\n", strings.Join(kubectlArgs, " "))
		}
		return nil
	}
	if f == nil {
		return runKubectl(kubectlArgs, opts)
	}
	// Pipe kubectl | grep.
	kCmd := exec.Command("kubectl", kubectlArgs...)
	gCmd := exec.Command("grep", f.Grep)
	pipe, err := kCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	gCmd.Stdin = pipe
	gCmd.Stdout = os.Stdout
	gCmd.Stderr = os.Stderr
	kCmd.Stderr = os.Stderr
	if err := kCmd.Start(); err != nil {
		return fmt.Errorf("start kubectl: %w", err)
	}
	if err := gCmd.Start(); err != nil {
		return fmt.Errorf("start grep: %w", err)
	}
	if err := kCmd.Wait(); err != nil {
		// Don't swallow grep's exit status if kubectl was fine.
		_ = gCmd.Wait()
		return fmt.Errorf("kubectl: %w", err)
	}
	return gCmd.Wait()
}

// runKubectlPatch patches a type/name resource in a namespace.
func runKubectlPatch(typeName, ns string, patch []byte, opts Options) error {
	args := []string{"patch", typeName, "-n", ns, "-p", string(patch), "--type", "merge"}
	return runKubectl(args, opts)
}

// runKubectlPatchType patches <rtype>/<name> in a namespace.
func runKubectlPatchType(rtype, name, ns string, patch []byte, opts Options) error {
	args := []string{"patch", rtype, name, "-n", ns, "-p", string(patch), "--type", "merge"}
	return runKubectl(args, opts)
}
