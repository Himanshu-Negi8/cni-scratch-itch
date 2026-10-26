package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"github.com/containernetworking/cni/pkg/skel"
	goipam "github.com/metal-stack/go-ipam"

	"github.com/Himanshu-Negi8/cni-scratch-itch/internal/models"
)

const (
	ipamStateDir  = "/var/lib/cni/cni-scratch-itch"
	ipamStateFile = ipamStateDir + "/ipam.json"
	ipamLockFile  = ipamStateDir + "/ipam.lock"
	networksDir   = "/var/lib/cni/networks"
)

// lockIPAM takes an exclusive, cross-process file lock so that concurrent
// ADD/DEL calls - each its own OS process under the CNI exec model - don't
// race on the same on-disk IPAM state.
func lockIPAM() (unlock func(), err error) {
	if err := os.MkdirAll(ipamStateDir, 0700); err != nil {
		return nil, fmt.Errorf("create ipam state dir: %w", err)
	}

	f, err := os.OpenFile(ipamLockFile, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// AllocateIP allocates an IP for the container from the subnet's pool and
// records the containerID that owns it under /var/lib/cni/networks/<name>/<ip>.
// CNI reserves stdout for the JSON result, so this must never print there -
// the caller is responsible for reporting the returned IP back to the runtime.
func AllocateIP(pc *models.PluginConf, args *skel.CmdArgs) (net.IP, error) {
	unlock, err := lockIPAM()
	if err != nil {
		return nil, fmt.Errorf("lock ipam state: %w", err)
	}
	defer unlock()

	ctx := context.Background()

	// File-backed storage: this binary is a fresh process per ADD/DEL, so
	// in-memory state would vanish the moment each call's process exits.
	ipam := goipam.NewWithStorage(goipam.NewLocalFile(ctx, ipamStateFile))

	// AcquireIP requires the prefix to already be registered in storage, so
	// ensure it exists first. NewPrefix errors on a self-overlap if the
	// prefix is already there, so fall back to a lookup in that case.
	if _, err := ipam.NewPrefix(ctx, pc.Subnet); err != nil {
		if _, lookupErr := ipam.PrefixFrom(ctx, pc.Subnet); lookupErr != nil {
			return nil, fmt.Errorf("ensure prefix %q: %w", pc.Subnet, err)
		}
	}

	// Reserve the bridge's gateway address in the pool so AcquireIP below
	// never hands it out to a container. Idempotent - already-reserved is fine.
	gw, err := GatewayForSubnet(pc.Subnet)
	if err != nil {
		return nil, fmt.Errorf("compute gateway for %q: %w", pc.Subnet, err)
	}
	if _, err := ipam.AcquireSpecificIP(ctx, pc.Subnet, gw.IP.String()); err != nil && !errors.Is(err, goipam.ErrAlreadyAllocated) {
		return nil, fmt.Errorf("reserve gateway ip %s: %w", gw.IP, err)
	}

	ip, err := ipam.AcquireIP(ctx, pc.Subnet)
	if err != nil {
		return nil, fmt.Errorf("acquire ip: %w", err)
	}

	networkDir := filepath.Join(networksDir, pc.Name)
	if err := os.MkdirAll(networkDir, 0700); err != nil {
		return nil, fmt.Errorf("create network dir %q: %w", networkDir, err)
	}

	filePath := filepath.Join(networkDir, ip.IP.String())
	if err := os.WriteFile(filePath, []byte(args.ContainerID), 0600); err != nil {
		if _, releaseErr := ipam.ReleaseIP(ctx, ip); releaseErr != nil {
			return nil, fmt.Errorf("write allocation file: %w (also failed to release ip: %v)", err, releaseErr)
		}
		return nil, fmt.Errorf("write allocation file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "allocated %s to container %s\n", ip.IP, args.ContainerID)

	return net.ParseIP(ip.IP.String()), nil
}
