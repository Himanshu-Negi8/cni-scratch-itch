package network

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
)

// EnsureBridge creates the named bridge if it doesn't exist, or returns the
// existing link if it does, and brings it up either way.
func EnsureBridge(name string) (*netlink.Bridge, error) {
	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   name,
			TxQLen: -1,
		},
	}

	if err := netlink.LinkAdd(br); err != nil && !errors.Is(err, syscall.EEXIST) {
		return nil, fmt.Errorf("could not create bridge %q: %w", name, err)
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("could not look up %q after creation: %w", name, err)
	}

	// ensuring it's a bridge not necessary as created in same scope
	bridge, ok := link.(*netlink.Bridge)
	if !ok {
		return nil, fmt.Errorf("%q already exists but is not a bridge", name)
	}

	if err := netlink.LinkSetUp(bridge); err != nil {
		return nil, fmt.Errorf("could not set %q up: %w", name, err)
	}

	return bridge, nil
}

// GatewayForSubnet returns the gateway address for a subnet: the first
// usable address, i.e. the network address + 1 (10.244.1.0/24 -> 10.244.1.1/24).
func GatewayForSubnet(subnet string) (*net.IPNet, error) {
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet %q: %w", subnet, err)
	}

	gw := make(net.IP, len(ipnet.IP))
	copy(gw, ipnet.IP)
	gw[len(gw)-1]++

	return &net.IPNet{IP: gw, Mask: ipnet.Mask}, nil
}

// EnsureBridgeAddress assigns the subnet's gateway address to the bridge if
// it doesn't already have it.
func EnsureBridgeAddress(br *netlink.Bridge, subnet string) error {
	gw, err := GatewayForSubnet(subnet)
	if err != nil {
		return err
	}

	addrs, err := netlink.AddrList(br, syscall.AF_INET)
	if err != nil {
		return fmt.Errorf("could not list addresses on %q: %w", br.Attrs().Name, err)
	}

	for _, addr := range addrs {
		if addr.IP.Equal(gw.IP) {
			return nil
		}
	}

	if err := netlink.AddrAdd(br, &netlink.Addr{IPNet: gw}); err != nil {
		return fmt.Errorf("could not add address %s to %q: %w", gw, br.Attrs().Name, err)
	}

	return nil
}
