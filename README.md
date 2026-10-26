# cni-scratch-itch

A CNI (Container Network Interface) plugin written in Go, built from scratch to
understand how pod networking actually works under the hood.

### Goal

Implement a minimal but spec-compliant CNI plugin binary, then validate it end-to-end
against a local `kind` cluster with the default CNI disabled. The kind cluster acts as
the test harness — kubelet invokes our binary directly for every pod `ADD`/`DEL`/`CHECK`
call, so it proves the plugin behaves correctly under real kubelet/containerd
orchestration rather than just unit tests.

Longer term, once the plugin follows the CNI spec correctly, evaluate whether it could
be dropped into other clusters as a lightweight replacement for the stock CNI.

### Current status

- **Implemented:** `ADD` for pod-to-pod connectivity on a **single node** (bridge + veth
  pairs + IP allocation).
- **Not yet implemented:** **cross-node** pod-to-pod connectivity (VXLAN overlay / static
  routes between nodes).
- **Not yet implemented:** the `CHECK` and `DEL` commands.

### Networking approach

The pod network is defined entirely in software (Linux bridge, veth pairs, addresses and
routes) through the [`vishvananda/netlink`](https://github.com/vishvananda/netlink) Go
library, which talks to the kernel over netlink — the same mechanism `ip link` /
`ip addr` / `ip route` use. The netlink reference is what we followed to create and
configure these virtual devices.

### How it will be tested

1. `kind-config.yaml` creates a 2-node cluster (`control-plane` + `worker`) with
   `networking.disableDefaultCNI: true`, so no CNI (kindnet, Calico, etc.) is installed.
   Nodes come up `NotReady` until a CNI is in place.
2. Our compiled plugin binary is copied into `/opt/cni/bin/` on each kind node, and a
   matching config is dropped into `/etc/cni/net.d/` on each node.
3. kubelet calls the binary (per the CNI spec: JSON on stdin, env vars for
   `CNI_COMMAND`/`CNI_CONTAINERID`/`CNI_NETNS`/etc.) whenever a pod sandbox is
   created or torn down.
4. Success is measured by: nodes transitioning to `Ready`, pods getting scheduled and
   reaching `Running` with an IP, and pod-to-pod / pod-to-service connectivity working
   as expected.

### Plan / building blocks

- [ ] Bootstrap the kind cluster (`kind create cluster --config kind-config.yaml`) and
      confirm nodes sit `NotReady` with no CNI.
- [ ] Implement the plugin skeleton using `containernetworking/cni` + `plugins`
      libraries (`skel.PluginMain`), handling `ADD`, `DEL`, and `CHECK`.
- [ ] Set up a Linux bridge (or point-to-point veth pairs) on each node for pod
      interfaces.
- [ ] Allocate pod IPs — start with a simple host-local/IPAM allocator, iterate later.
- [ ] Wire up routes so pods can reach each other across nodes (may start
      single-node/no cross-node routing, then layer in a simple overlay or static
      routes).
- [ ] Load the built binary + config into the kind nodes and confirm kubelet invokes it
      successfully (`crictl`/kubelet logs, `kubectl get nodes` going `Ready`).
- [ ] Validate with real workloads: deploy pods, check connectivity, clean up on pod
      deletion (`DEL` releases the IP/veth correctly, no leaks).

### Repo layout (so far)

```
.
├── go.mod            # module: github.com/Himanshu-Negi8/cni-scratch-itch
├── kind-config.yaml  # kind cluster with the default CNI disabled
└── README.md
```

The plugin binary's source will live under a `cmd/` or top-level package as the
implementation grows.

### References

- CNI spec: https://github.com/containernetworking/cni/blob/main/SPEC.md
- Reference plugins (bridge, host-local IPAM, ptp, etc.):
  https://github.com/containernetworking/plugins
- netlink Go library (used to define the software network): https://github.com/vishvananda/netlink
- kind networking docs: https://kind.sigs.k8s.io/docs/user/configuration/#networking
