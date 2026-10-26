package network

import (
	"fmt"
	"net"

	"github.com/Himanshu-Negi8/cni-scratch-itch/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

// CreateVethPair wires up a veth pair for the container: the host end is
// enslaved to br so it can reach the bridge/gateway, and the container end
// (eth0) is given containerIP/subnet plus a default route via gw - a bare
// "up" link with no address and no bridge membership can't carry traffic.
func CreateVethPair(args *skel.CmdArgs, br *netlink.Bridge, containerIP net.IP, subnet *net.IPNet, gw net.IP) error {
	hostVethName, tmpPeerName, finalContainerVethName := generateVethNames(args.ContainerID)

	veth := netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: hostVethName,
		},
		PeerName: tmpPeerName,
	}

	// since binary is running in the node so both pairs are created in the node
	if err := netlink.LinkAdd(&veth); err != nil {
		return err
	}

	// the host end also needs to be brought up, otherwise the pair never carries traffic
	if err := netlink.LinkSetUp(&veth); err != nil {
		return err
	}

	// enslave the host end to the bridge - without this it's a standalone
	// link with no L2 path to the bridge (or the gateway IP living on it)
	if err := netlink.LinkSetMaster(&veth, br); err != nil {
		return err
	}

	peerLink, err := netlink.LinkByName(tmpPeerName)
	if err != nil {
		return err
	}

	targetNS, err := ns.GetNS(args.Netns)
	if err != nil {
		return err
	}
	defer targetNS.Close()

	// 5. Push the peer link into the container namespace
	if err := netlink.LinkSetNsFd(peerLink, int(targetNS.Fd())); err != nil {
		return err
	}

	// 6. Enter the container namespace to rename it, address it, and bring it up
	err = targetNS.Do(func(_ ns.NetNS) error {
		// Find the peer by its temporary name INSIDE the container namespace
		containerVeth, err := netlink.LinkByName(tmpPeerName)
		if err != nil {
			return err
		}

		// Rename it from "veth-pod1-peer" to "eth0"
		if err := netlink.LinkSetName(containerVeth, finalContainerVethName); err != nil {
			return err
		}

		// Bring it up
		if err := netlink.LinkSetUp(containerVeth); err != nil {
			return err
		}

		// Assign the allocated IP - a link with no address can't source a ping
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: containerIP, Mask: subnet.Mask}}
		if err := netlink.AddrAdd(containerVeth, addr); err != nil {
			return err
		}

		// Route everything else through the bridge's gateway address
		route := &netlink.Route{
			LinkIndex: containerVeth.Attrs().Index,
			Gw:        gw,
		}

		return netlink.RouteAdd(route)
	})
	return err
}

func generateVethNames(containerID string) (string, string, string) {
	// Linux caps interface names at IFNAMSIZ (15 chars). The full MD5 hex
	// hash is 32 chars, which overflows that limit once prefixed with
	// "veth-"/"peer-" and makes netlink.LinkAdd/LinkSetName fail. Truncate
	// to 8 hex chars (32 bits) - collisions are not a concern since this
	// only needs to be unique among a single node's concurrently running pods.
	hash := utils.GenerateHash(containerID)[:8]

	hostVethName := fmt.Sprintf("veth-%s", hash)
	tmpPeerName := fmt.Sprintf("peer-%s", hash)
	finalContainerVethName := fmt.Sprintf("veth-%s", hash)

	return hostVethName, tmpPeerName, finalContainerVethName
}
