package libpodruntime

import (
	"context"

	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/containers/libpod/pkg/util"
	"github.com/containers/storage"
	"github.com/pkg/errors"
)

// GetRuntimeMigrate gets a libpod runtime that will perform a migration of existing containers
func GetRuntimeMigrate(ctx context.Context, c *cliconfig.PodmanCommand) (*libpod.Runtime, error) {
	return getRuntime(ctx, c, false, true)
}

// GetRuntimeRenumber gets a libpod runtime that will perform a lock renumber
func GetRuntimeRenumber(ctx context.Context, c *cliconfig.PodmanCommand) (*libpod.Runtime, error) {
	return getRuntime(ctx, c, true, false)
}

// GetRuntime generates a new libpod runtime configured by command line options
func GetRuntime(ctx context.Context, c *cliconfig.PodmanCommand) (*libpod.Runtime, error) {
	return getRuntime(ctx, c, false, false)
}

func getRuntime(ctx context.Context, c *cliconfig.PodmanCommand, renumber bool, migrate bool) (*libpod.Runtime, error) {
	options := []libpod.RuntimeOption{}
	storageOpts := storage.StoreOptions{}
	storageSet := false

	uidmapFlag := c.Flags().Lookup("uidmap")
	gidmapFlag := c.Flags().Lookup("gidmap")
	subuidname := c.Flags().Lookup("subuidname")
	subgidname := c.Flags().Lookup("subgidname")
	if (uidmapFlag != nil && gidmapFlag != nil && subuidname != nil && subgidname != nil) &&
		(uidmapFlag.Changed || gidmapFlag.Changed || subuidname.Changed || subgidname.Changed) {
		uidmapVal, _ := c.Flags().GetStringSlice("uidmap")
		gidmapVal, _ := c.Flags().GetStringSlice("gidmap")
		subuidVal, _ := c.Flags().GetString("subuidname")
		subgidVal, _ := c.Flags().GetString("subgidname")
		mappings, err := util.ParseIDMapping(uidmapVal, gidmapVal, subuidVal, subgidVal)
		if err != nil {
			return nil, err
		}
		storageOpts.UIDMap = mappings.UIDMap
		storageOpts.GIDMap = mappings.GIDMap

		storageSet = true
	}

	if c.Flags().Changed("root") {
		storageSet = true
		storageOpts.GraphRoot = c.GlobalFlags.Root
	}
	if c.Flags().Changed("runroot") {
		storageSet = true
		storageOpts.RunRoot = c.GlobalFlags.Runroot
	}
	if len(storageOpts.RunRoot) > 50 {
		return nil, errors.New("the specified runroot is longer than 50 characters")
	}
	if c.Flags().Changed("storage-driver") {
		storageSet = true
		storageOpts.GraphDriverName = c.GlobalFlags.StorageDriver
	}
	if len(c.GlobalFlags.StorageOpts) > 0 {
		storageSet = true
		storageOpts.GraphDriverOptions = c.GlobalFlags.StorageOpts
	}
	if migrate {
		options = append(options, libpod.WithMigrate())
	}

	if renumber {
		options = append(options, libpod.WithRenumber())
	}

	// Only set this if the user changes storage config on the command line
	if storageSet {
		options = append(options, libpod.WithStorageConfig(storageOpts))
	}

	// TODO CLI flags for image config?
	// TODO CLI flag for signature policy?

	if len(c.GlobalFlags.Namespace) > 0 {
		options = append(options, libpod.WithNamespace(c.GlobalFlags.Namespace))
	}

	if c.Flags().Changed("runtime") {
		options = append(options, libpod.WithOCIRuntime(c.GlobalFlags.Runtime))
	}

	if c.Flags().Changed("conmon") {
		options = append(options, libpod.WithConmonPath(c.GlobalFlags.ConmonPath))
	}
	if c.Flags().Changed("tmpdir") {
		options = append(options, libpod.WithTmpDir(c.GlobalFlags.TmpDir))
	}
	if c.Flags().Changed("network-cmd-path") {
		options = append(options, libpod.WithNetworkCmdPath(c.GlobalFlags.NetworkCmdPath))
	}

	if c.Flags().Changed("cgroup-manager") {
		options = append(options, libpod.WithCgroupManager(c.GlobalFlags.CGroupManager))
	} else {
		if rootless.IsRootless() {
			options = append(options, libpod.WithCgroupManager("cgroupfs"))
		}
	}

	// TODO flag to set libpod static dir?
	// TODO flag to set libpod tmp dir?

	if c.Flags().Changed("cni-config-dir") {
		options = append(options, libpod.WithCNIConfigDir(c.GlobalFlags.CniConfigDir))
	}
	if c.Flags().Changed("default-mounts-file") {
		options = append(options, libpod.WithDefaultMountsFile(c.GlobalFlags.DefaultMountsFile))
	}
	if c.Flags().Changed("hooks-dir") {
		options = append(options, libpod.WithHooksDir(c.GlobalFlags.HooksDir...))
	}

	// TODO flag to set CNI plugins dir?

	// TODO I dont think these belong here?
	// Will follow up with a different PR to address
	//
	// Pod create options

	infraImageFlag := c.Flags().Lookup("infra-image")
	if infraImageFlag != nil && infraImageFlag.Changed {
		infraImage, _ := c.Flags().GetString("infra-image")
		options = append(options, libpod.WithDefaultInfraImage(infraImage))
	}

	infraCommandFlag := c.Flags().Lookup("infra-command")
	if infraCommandFlag != nil && infraImageFlag.Changed {
		infraCommand, _ := c.Flags().GetString("infra-command")
		options = append(options, libpod.WithDefaultInfraCommand(infraCommand))
	}
	if c.Flags().Changed("config") {
		return libpod.NewRuntimeFromConfig(ctx, c.GlobalFlags.Config, options...)
	}
	return libpod.NewRuntime(ctx, options...)
}
