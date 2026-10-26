package models

import "github.com/containernetworking/cni/pkg/types"

type PluginConf struct {
	types.NetConf
	Bridge string `json:"bridge"`
	Subnet string `json:"subnet"`
}
