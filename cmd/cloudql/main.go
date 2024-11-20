package main

import (
	"github.com/opengovern/opengovernance/pkg/cloudql/tables"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{PluginFunc: opengovernance.Plugin})
}