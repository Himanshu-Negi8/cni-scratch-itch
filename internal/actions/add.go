package actions

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/Himanshu-Negi8/cni-scratch-itch/internal/models"
	"github.com/Himanshu-Negi8/cni-scratch-itch/internal/network"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
)

// this file contains the code for adding the container to the network

func AddContainerToNetwork(args *skel.CmdArgs) error {
	pc := &models.PluginConf{}

	err := json.Unmarshal(args.StdinData, &pc)
	if err != nil {
		return fmt.Errorf("failed to parse network config: %w", err)
	}

	// 1. We need to create a bridge on node if it doesn't exist
	br, err := network.EnsureBridge(pc.Bridge)
	if err != nil {
		return fmt.Errorf("failed to ensure bridge %q: %w", pc.Bridge, err)
	}

	// 1a. Give the bridge the subnet's gateway address so it can be excluded
	// from the IP pool handed out to containers
	if err := network.EnsureBridgeAddress(br, pc.Subnet); err != nil {
		return fmt.Errorf("failed to set gateway address on bridge %q: %w", pc.Bridge, err)
	}

	// 2. We need to get acquire an ip from a pool and write it to
	// /var/lib/cni/cni-scratch-itch/<network-name>/containerid
	ip, err := network.AllocateIP(pc, args)
	if err != nil {
		return fmt.Errorf("failed to allocate IP: %w", err)
	}

	_, subnet, err := net.ParseCIDR(pc.Subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet %q: %w", pc.Subnet, err)
	}

	gw, err := network.GatewayForSubnet(pc.Subnet)
	if err != nil {
		return fmt.Errorf("failed to compute gateway for %q: %w", pc.Subnet, err)
	}

	// 3. We need to create veth pair now one end on containers other is on bridge
	// Connect them so it can ping nodes
	err = network.CreateVethPair(args, br, ip, subnet, gw.IP)
	if err != nil {
		return fmt.Errorf("failed to create veth pair: %w", err)
	}

	// CNI reserves stdout for this JSON result - the runtime parses it to
	// learn the assigned IP, so nothing else may be printed to stdout.
	result := &types100.Result{
		CNIVersion: pc.CNIVersion,
		IPs: []*types100.IPConfig{
			{
				Address: net.IPNet{IP: ip, Mask: subnet.Mask},
				Gateway: gw.IP,
			},
		},
	}

	fmt.Fprintf(os.Stderr, "add: assigned %s to container %s\n", ip, args.ContainerID)

	return types.PrintResult(result, pc.CNIVersion)
}
