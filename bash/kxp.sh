# bash/kxp.sh — v0 reference implementation of KubeXPander in pure bash.
#
# This is the original prototype the Go binary replaces. It works without
# compiling anything — just `source bash/kxp.sh` from your ~/.bashrc.
# The Go binary (`kxp install`) generates a thinner version of this that
# delegates back to `kxp dispatch` for all the smarts.
#
# Terminology: a "slug" is a short namespace alias. `kclo` dispatches to the
# `cloudflare` namespace — `k` is the prefix, `clo` is the slug.
#
# Behavior:
#   kclo                       -> kubectl get pods -n cloudflare
#   kclo o                     -> kubectl get pods -n cloudflare -o wide
#   kclo o yaml                -> kubectl get pods -n cloudflare -o yaml
#   kclo lsf                    -> list resources with finalizers in cloudflare
#   kclo rmf deployment foo     -> strip finalizers from deployment/foo
#   kclo rmf deployment foo bar -> strip finalizers from multiple
#   kclo rmf deployment/foo     -> single type/name form
#   kclo get deploy            -> kubectl -n cloudflare get deploy (pass-through)
#   kclo delete pod foo        -> kubectl -n cloudflare delete pod foo (pass-through)
#
#   kcil                       -> kube-system pods | grep cilium (filtered)
#   kcil o                     -> ... -o wide | grep cilium
#   kcil rmf pod foo            -> strip finalizers (NOT filtered — patching is deliberate)
#   kcil delete pod foo        -> pass-through (NOT filtered)
#
#   kall                       -> kubectl get pods --all-namespaces
#   kall o                     -> ... -o wide
#   kall lsf                    -> whole-cluster finalizer scan
#   kall rmf ...                -> INTENTIONALLY UNSUPPORTED (too easy to misfire)
#
#   lsf                         -> prints usage (footgun guard)

declare -A K8S_NS_SLUGS=()

# Resource types worth scanning for finalizers. Curated list keeps it fast and
# predictable; add/remove as needed. Split into namespaced vs cluster-scoped.
#
# ingress is included deliberately: on clusters using Cilium Gateway API in
# place of a conventional ingress controller, scanning for finalizers on
# ingress objects catches cases where an AI agent (or a distracted human) has
# added an ingress-type object that the Gateway API controller doesn't
# reconcile, leaving it stuck terminating.
_K8S_FIN_TYPES_NS="pods deployments statefulsets daemonsets replicasets \
persistentvolumeclaims services ingress configmaps secrets"
_K8S_FIN_TYPES_CLUSTER="namespaces persistentvolumes"

# Print every resource that currently has finalizers set.
#   _k8s_list_finalizers ""            -> whole cluster (all namespaces + cluster-scoped)
#   _k8s_list_finalizers cloudflare    -> just that namespace
_k8s_list_finalizers() {
  local ns="$1" rtype
  if [[ -n "$ns" ]]; then
    for rtype in $_K8S_FIN_TYPES_NS; do
      kubectl get "$rtype" -n "$ns" -o \
        jsonpath='{range .items[?(@.metadata.finalizers)]}{.kind}{"/"}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}' \
        2>/dev/null
    done
    return
  fi
  for rtype in $_K8S_FIN_TYPES_NS; do
    kubectl get "$rtype" --all-namespaces -o \
      jsonpath='{range .items[?(@.metadata.finalizers)]}{.metadata.namespace}{"/"}{.kind}{"/"}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}' \
      2>/dev/null
  done
  for rtype in $_K8S_FIN_TYPES_CLUSTER; do
    kubectl get "$rtype" -o \
      jsonpath='{range .items[?(@.metadata.finalizers)]}{.kind}{"/"}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}' \
      2>/dev/null
  done
}

# Whole-cluster finalizer scan is intentionally NOT bound to bare `lsf` —
# too easy to fire by accident. Use `kall lsf` for that.
lsf(){
  echo "Usage: kall lsf                       (whole-cluster scan)" >&2
  echo "       k<slug> lsf                    (e.g. kclo lsf — single namespace)" >&2
  return 1
}

kubectl_slug() {
  local caller namespace
  caller="${FUNCNAME[1]}"
  namespace="${K8S_NS_SLUGS[$caller]}"
  if [[ -z "$namespace" ]]; then
    echo "No namespace mapped for $caller" >&2
    return 1
  fi

  if [[ $# -eq 0 ]]; then
    kubectl get pods -n "$namespace"
    return
  fi

  if [[ $1 == o ]]; then
    shift
    if [[ $# -eq 0 ]]; then
      kubectl get pods -n "$namespace" -o wide
    else
      kubectl get pods -n "$namespace" -o "$@"
    fi
    return
  fi

  # Strip finalizers from a resource so it can finish deleting.
  #   kclo rmf deployment cloudflared-dex   -> patch deployment/cloudflared-dex
  #   kclo rmf pod foo bar                  -> patch multiple names
  #   kclo rmf deployment/cloudflared-dex   -> type/name single-arg form
  if [[ $1 == rmf ]]; then
    shift
    if [[ $# -lt 1 ]]; then
      echo "Usage: $caller rmf <type> <name> [name...]  |  $caller rmf <type/name>" >&2
      return 1
    fi
    read -rp "Are you sure you want to remove finalizers from $* in namespace $namespace? [y/N] " ans
    if [[ "${ans,,}" != "y" && "${ans,,}" != "yes" ]]; then
      echo "rmf: cancelled." >&2
      return 1
    fi
    if [[ $# -eq 1 && "$1" == */* ]]; then
      kubectl patch "$1" -n "$namespace" \
        -p '{"metadata":{"finalizers":null}}' --type merge
      return
    fi
    if [[ $# -lt 2 ]]; then
      echo "Usage: $caller rmf <type> <name> [name...]" >&2
      return 1
    fi
    local rtype="$1"; shift
    local name
    for name in "$@"; do
      kubectl patch "$rtype" "$name" -n "$namespace" \
        -p '{"metadata":{"finalizers":null}}' --type merge
    done
    return
  fi

  # List resources in this namespace that currently have finalizers set.
  #   kclo lsf   -> scan cloudflare namespace for finalizers
  if [[ $1 == lsf ]]; then
    _k8s_list_finalizers "$namespace"
    return
  fi

  kubectl -n "$namespace" "$@"
}

add_k8s_slug() {
  local alias="$1" ns="$2"
  K8S_NS_SLUGS["$alias"]="$ns"
  eval "$alias(){ kubectl_slug \"\$@\"; }"
}

# --- Reference namespace map (edit to match your cluster) ---
add_k8s_slug kpxe pxe-boot
add_k8s_slug ksys kube-system
add_k8s_slug kred redis
add_k8s_slug kpub kube-public
add_k8s_slug kclo cloudflare
add_k8s_slug kcwa calibre-cwa
add_k8s_slug kgol goldilocks
add_k8s_slug kimg image-pull
add_k8s_slug khat hatchet
add_k8s_slug klease kube-node-lease
add_k8s_slug kflowd kflowd
add_k8s_slug ktfstate tofu-state
add_k8s_slug kcert cert-manager
add_k8s_slug khom homarr
add_k8s_slug kcnpg cnpg
add_k8s_slug kargo argocd
add_k8s_slug kinf infisical
add_k8s_slug klin linear
add_k8s_slug kgraf grafana
add_k8s_slug ktempo tempo
add_k8s_slug ktem temporal
add_k8s_slug kwf temporal
add_k8s_slug kzot zot
add_k8s_slug kdef default
add_k8s_slug kfor forgejo
add_k8s_slug kfal falco
add_k8s_slug kmet metrics-exporters
add_k8s_slug kcar caretta
add_k8s_slug kobi obi
add_k8s_slug kotel opentelemetry
add_k8s_slug kdex auth
add_k8s_slug ktet tetragon
add_k8s_slug kpyr pyroscope
add_k8s_slug klok loki
add_k8s_slug kalloy alloy
add_k8s_slug kpor portainer
add_k8s_slug kolm olm
add_k8s_slug kvic victoriametrics
add_k8s_slug kvel velero
add_k8s_slug kren renovate
add_k8s_slug krce rook-ceph-external
add_k8s_slug krc rook-ceph

king(){ kubectl get ingress --all-namespaces "$@"; }

# Cluster-scoped getters (kgn, kns). Not namespace slugs — they have no -n flag.
# The `o` verb is handled like namespace slugs; everything else passes through
# to kubectl without a `get <resource>` prefix so describe/label/edit work.
#
#   kgn                   -> kubectl get nodes
#   kgn o                 -> kubectl get nodes -o wide
#   kgn o yaml            -> kubectl get nodes -o yaml
#   kgn describe node foo -> kubectl describe node foo (pass-through)
#   kns                   -> kubectl get namespaces
#   kns o                 -> kubectl get namespaces -o wide
_k8s_cluster_get() {
  local resource="$1"; shift
  if [[ $# -eq 0 ]]; then
    kubectl get "$resource"
    return
  fi
  if [[ $1 == o ]]; then
    shift
    if [[ $# -eq 0 ]]; then
      kubectl get "$resource" -o wide
    else
      kubectl get "$resource" -o "$@"
    fi
    return
  fi
  kubectl "$@"
}
kgn(){ _k8s_cluster_get nodes "$@"; }
kns(){ _k8s_cluster_get namespaces "$@"; }

# Like kubectl_slug but applies a grep filter ONLY to the default pod listing
# (and its `o` variant). Everything else (rmf, delete, get deploy, ...) passes
# through to kubectl unfiltered so the slug works like the namespace ones.
kubectl_filtered_ns() {
  local namespace="$1" pattern="$2"
  shift 2
  local caller="${FUNCNAME[1]}"

  if [[ $# -eq 0 ]]; then
    kubectl get pods -n "$namespace" | grep "$pattern"
    return
  fi

  if [[ $1 == o ]]; then
    shift
    if [[ $# -eq 0 ]]; then
      kubectl get pods -n "$namespace" -o wide | grep "$pattern"
    else
      kubectl get pods -n "$namespace" -o "$@" | grep "$pattern"
    fi
    return
  fi

  if [[ $1 == rmf ]]; then
    shift
    if [[ $# -lt 1 ]]; then
      echo "Usage: $caller rmf <type> <name> [name...]  |  $caller rmf <type/name>" >&2
      return 1
    fi
    read -rp "Are you sure you want to remove finalizers from $* in namespace $namespace? [y/N] " ans
    if [[ "${ans,,}" != "y" && "${ans,,}" != "yes" ]]; then
      echo "rmf: cancelled." >&2
      return 1
    fi
    if [[ $# -eq 1 && "$1" == */* ]]; then
      kubectl patch "$1" -n "$namespace" \
        -p '{"metadata":{"finalizers":null}}' --type merge
      return
    fi
    if [[ $# -lt 2 ]]; then
      echo "Usage: $caller rmf <type> <name> [name...]" >&2
      return 1
    fi
    local rtype="$1"; shift
    local name
    for name in "$@"; do
      kubectl patch "$rtype" "$name" -n "$namespace" \
        -p '{"metadata":{"finalizers":null}}' --type merge
    done
    return
  fi

  if [[ $1 == lsf ]]; then
    _k8s_list_finalizers "$namespace"
    return
  fi

  kubectl -n "$namespace" "$@"
}

kcil(){ kubectl_filtered_ns kube-system cilium "$@"; }
kenv(){ kubectl_filtered_ns kube-system envoy "$@"; }

# All-namespaces variant. Only `o` (-> -o wide), `lsf` (whole-cluster
# finalizer scan), and raw pass-through; no `rmf` here on purpose — patching
# across namespaces is too easy to misfire.
kall(){
  if [[ $# -eq 0 ]]; then
    kubectl get pods --all-namespaces
    return
  fi
  if [[ $1 == o ]]; then
    shift
    if [[ $# -eq 0 ]]; then
      kubectl get pods --all-namespaces -o wide
    else
      kubectl get pods --all-namespaces -o "$@"
    fi
    return
  fi
  if [[ $1 == lsf ]]; then
    _k8s_list_finalizers ""
    return
  fi
  kubectl "$@"
}
