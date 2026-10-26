package main

import (
	"fmt"
	"io"

	"github.com/Himanshu-Negi8/cni-scratch-itch/internal/actions"
	"github.com/containernetworking/cni/pkg/skel"
)

type pluginInfo struct {
}

func (p *pluginInfo) SupportedVersions() []string {
	return []string{"1.0.0"}
}

func (p *pluginInfo) Encode(w io.Writer) error {
	return nil
}

// Root Command
func main() {
	funcs := skel.CNIFuncs{
		Add:   actions.AddContainerToNetwork,
		Del:   actions.RemoveContainerFromNetwork,
		Check: actions.CheckContainerNetworking,
	}

	plugin := &pluginInfo{}
	err := skel.PluginMainFuncsWithError(funcs, plugin, "custom plugin for kind")
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
}
