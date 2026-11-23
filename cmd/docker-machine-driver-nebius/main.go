package main

import (
	"github.com/gorizond/docker-machine-driver-nebius/nebius"
	"github.com/docker/machine/libmachine/drivers/plugin"
)

func main() {
	plugin.RegisterDriver(nebius.NewDriver("", ""))
}
