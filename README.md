# KubeXPander

Reduce wasted keystrokes interacting with Kubernetes.

`kclo o` instead of `kubectl get pods -n cloudflare -o wide`.

`kclo rmf deployment cloudflared-dex` instead of the finalizer patch incantation.

## What it does

KubeXPander generates short shell functions (`kclo`, `ksys`, `kcnpg`, ...) that
each map to a Kubernetes namespace and dispatch to `kubectl` with the right
`-n` flag. On top of plain pass-through, three verbs get first-class support:

| Verb | Meaning |
|------|---------|
| `o`  | `-o wide` (or custom `-o` format if more args follow) |
| `lsf` | list resources with finalizers set in the namespace |
| `rmf` | strip finalizers from a resource so it can finish deleting |

Anything else is passed straight to `kubectl -n <ns>`, so `kclo delete pod foo`
and `kclo get deploy` work exactly as you'd expect.

Everything after the slug name belongs to `kubectl`, including flags — so
`kclo delete pod foo --force --grace-period=0` and `kclo logs foo -f` work.
The flip side is that kxp's own flags (`--dry-run`, `--config`) must come
*before* the slug name:

```bash
kxp --dry-run dispatch clo delete pod foo --force   # kxp flag first: works
kxp dispatch clo delete pod foo --dry-run           # --dry-run goes to kubectl
```

### Built-in cluster-scoped getters: `kgn` and `kns`

Two cluster-scoped shortcuts ship built-in (no namespace, no config entry
needed):

| Shortcut | Meaning |
|----------|---------|
| `kgn`  | `kubectl get nodes` |
| `kns`  | `kubectl get namespaces` |

Both support the `o` verb (`kgn o` -> `kubectl get nodes -o wide`) and pass
anything else straight to `kubectl` (e.g. `kgn describe node foo` ->
`kubectl describe node foo`). User slugs always take precedence — if your
config defines a slug named `gn` or `ns`, the slug wins and the built-in is
shadowed.

## Prerequisites

- **A Kubernetes cluster** you can reach. KubeXPander constructs the full
  `kubectl` commands corresponding to what you give it via the `k<slug>`
  shorthand form, then runs them. Whatever cluster your `kubectl` points at
  is the one the slugs hit.
- **`kubectl` on `$PATH`, working without `sudo`.** The generated functions
  call `kubectl` directly. If your `kubectl` requires `sudo` (e.g.
  `sudo kubectl` on microk8s without the microk8s.kubectl alias), either fix
  that (e.g. `sudo usermod -aG microk8s $USER` and re-login, or add
  `alias kubectl='sudo kubectl'` to your `~/.bashrc`) or use the bash
  reference (`bash/kxp.sh`) which you can edit to prefix `sudo` where needed.
- **Linux.** Tested on Ubuntu 26.04 LTS. The Go binary itself is cross-compilable, but
  the generated shell functions assume bash and the `complete` builtin, which
  is Linux/WSL territory. macOS *might* work with `bash-completion` installed
  (bash 3.2 ships with macOS and lacks `_init_completion`; the completion
  block degrades gracefully but won't be as smart). Not tested on macOS thus far, but it probably will be eventually.
- **bash** for the generated shell functions. zsh is not supported by the
  generated snippet (the `complete` builtin is bash-specific); if you're on
  zsh, use the Go binary directly via `kxpd <slug>` instead of the `k<slug>`
  functions.
- **Go 1.25+** if building from source (only needed for the Go binary; the
  bash reference has no build step).

## Why "KubeXPander"?

Because that's what it does: **expands** a short slug into the full `kubectl`
invocation. `kclo o` expands to `kubectl get pods -n cloudflare -o wide`.

The default 3-8 character length range for slug names is the sweet spot:
short enough to type fast, long enough to be memorable and unique. `k` alone
is too short to be unambiguous; `kubectl delete replicaset cloudflared-backchannel -n cloudflared-system`
is too long to type more often than the once I just did. The range can be
overridden (`allowShorter`/`allowLonger`) in the config.

## What's a "slug"?

A **slug** is a short namespace alias. `kclo` dispatches to the `cloudflare` namespace — `k` is the prefix, `clo` is the slug.

## Two ways to use it:

### 1. Go binary (recommended)

**One-line install:**

```bash
curl -fsSL https://raw.githubusercontent.com/weefarm/KubeXPander/main/install.sh | bash
```

This installs the `kxp` binary via `go install`, ensures it's on your `$PATH`, and writes a starter config to `~/.config/kxp/slugs.yaml`. Safe to re-run; does not require `sudo` (and will refuse to run as root).

After the install script finishes, edit your config and wire up the shell functions:

```bash
$EDITOR ~/.config/kxp/slugs.yaml   # edit to match your namespaces
kxp install >> ~/.bashrc           # emit shell functions + completions + kxpd alias
source ~/.bashrc
```

**Manual install** (if you prefer to run the steps yourself):

```bash
go install github.com/weefarm/KubeXPander/cmd/kxp@latest
# go install puts the binary in $(go env GOPATH)/bin — add it to PATH:
export PATH="$PATH:$(go env GOPATH)/bin"
# To make the PATH change permanent, append it to your shell rc:
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc
kxp init                       # writes ~/.config/kxp/slugs.yaml
$EDITOR ~/.config/kxp/slugs.yaml   # edit to match your namespaces
kxp install >> ~/.bashrc       # emit shell functions + completions + kxpd alias
source ~/.bashrc
```

> **Note:** `go install` only compiles and places the binary in `$GOPATH/bin`
> (typically `~/go/bin`). It does **not** run `kxp init` or modify your shell
> config. On a fresh box `~/go/bin` is not on `$PATH` by default, so `kxp`
> will be "command not found" until you add it. The one-line `install.sh`
> installer above handles this automatically; for manual installs you must
> add `$GOPATH/bin` to `$PATH` yourself before `kxp init` will work.

The generated functions are thin wrappers that call back into `kxp dispatch`:

```bash
kclo(){ kxp dispatch clo "$@"; }
ksys(){ kxp dispatch sys "$@"; }
alias kxpd="kxp dispatch"  # alternate invocation: kxpd clo == kclo
```

All the dispatch logic lives in the binary; the shell functions are dumb pipes.
This means tab completion, `--dry-run`, structured output, and tests come for
free, and there's no multi-line noise copy-pasted into every function to clutter
your .profile or .bashrc.

### 2. Bash reference (no compilation)

```bash
source bash/kxp.sh             # from this repo
```
`bash/kxp.sh` is the v0 prototype the Go binary replaces. It works without
compiling anything — just source it from your shell or as in include in your
`~/.bashrc`. 

Edit the `add_k8s_slug` lines at the bottom to assign slugs to your namespaces.

## Config file

`~/.config/kxp/slugs.yaml` (or `$XDG_CONFIG_HOME/kxp/slugs.yaml`):

```yaml
slugs:
  clo: cloudflare
  sys: kube-system
  cnpg: cnpg

filtered:                       # grep-filtered pod listings (kcil/kenv style)
  cil:
    ns: kube-system
    grep: cilium

allSlug: all                   # the all-namespaces slug name; provides "kall" by default.  if you changed it to "any" then "kany" would correspond to "kubectl get pods --all-namespaces"

allowShorter: true              # allow 1-2 char slugs (default: min 3)
# allowLonger: true             # allow 9+ char slugs (default: max 8)
```

Override the config path with `--config PATH` on any `kxp` subcommand.

## Verbs in detail

### `o` — output format

```
kclo            # kubectl get pods -n cloudflare
kclo o          # kubectl get pods -n cloudflare -o wide
kclo o yaml     # kubectl get pods -n cloudflare -o yaml
```

### `lsf` — list finalizers

Lists every resource in the namespace that has `metadata.finalizers` set.
Output: `Kind/name\t[finalizers...]`.

```
kclo lsf         # scan cloudflare namespace for finalizers
kall lsf         # whole-cluster scan (all namespaces + cluster-scoped)
lsf              # prints usage (footgun guard — too easy to fire by accident)
```

The scan covers a curated type list: pods, deployments, statefulsets,
daemonsets, replicasets, persistentvolumeclaims, services, ingress,
configmaps, secrets (namespaced) and namespaces, persistentvolumes
(cluster-scoped). `ingress` is included deliberately — on clusters using
Cilium Gateway API in place of a conventional ingress controller, scanning
for finalizers on ingress objects catches cases where an AI agent (or a
distracted human) has added an ingress-type object that the Gateway API
controller doesn't reconcile, leaving it stuck terminating.

### `rmf` — remove finalizers

Patches `metadata.finalizers` to `null` so a stuck resource can finish
terminating. NOT filtered even on filtered slugs — patching is a deliberate
act and grep would obscure the target.

A **confirmation prompt** gates the actual dispatch: the user is asked
"Are you sure you want to remove finalizers from ...?" and must type `y` or
`yes` to proceed. Anything else cancels the operation. The prompt is skipped
in `--dry-run` mode.

```
kclo rmf deployment foo           # patch deployment/foo in cloudflare
kclo rmf deployment foo bar       # patch multiple
kclo rmf deployment/foo           # type/name single-arg form
```

`kall rmf` is intentionally unsupported — patching across namespaces is too
easy to misfire. Use a namespace-scoped slug instead.

## Alternate invocation: `kxpd`

`kxp install` also emits `alias kxpd="kxp dispatch"`, so you can do:

```
kxpd clo            # == kclo  == kxp dispatch clo
kxpd clo o          # == kclo o
kxpd clo rmf deployment foo
```

Useful if you prefer an explicit command form over the generated `k<slug>`
functions, or in contexts where the functions aren't loaded (e.g. scripts
that just call `kxp` directly).

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
