package main

import (
	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/adapter"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	restoreCommand     cliconfig.RestoreValues
	restoreDescription = `
   podman container restore

   Restores a container from a checkpoint. The container name or ID can be used.
`
	_restoreCommand = &cobra.Command{
		Use:   "restore [flags] CONTAINER [CONTAINER...]",
		Short: "Restores one or more containers from a checkpoint",
		Long:  restoreDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			restoreCommand.InputArgs = args
			restoreCommand.GlobalFlags = MainGlobalOpts
			restoreCommand.Remote = remoteclient
			return restoreCmd(&restoreCommand)
		},
		Args: func(cmd *cobra.Command, args []string) error {
			return checkAllAndLatest(cmd, args, false)
		},
		Example: `podman container restore ctrID
  podman container restore --latest
  podman container restore --all`,
	}
)

func init() {
	restoreCommand.Command = _restoreCommand
	restoreCommand.SetHelpTemplate(HelpTemplate())
	restoreCommand.SetUsageTemplate(UsageTemplate())
	flags := restoreCommand.Flags()
	flags.BoolVarP(&restoreCommand.All, "all", "a", false, "Restore all checkpointed containers")
	flags.BoolVarP(&restoreCommand.Keep, "keep", "k", false, "Keep all temporary checkpoint files")
	flags.BoolVarP(&restoreCommand.Latest, "latest", "l", false, "Act on the latest container podman is aware of")
	// TODO: add ContainerStateCheckpointed
	flags.BoolVar(&restoreCommand.TcpEstablished, "tcp-established", false, "Checkpoint a container with established TCP connections")

	markFlagHiddenForRemoteClient("latest", flags)
}

func restoreCmd(c *cliconfig.RestoreValues) error {
	if rootless.IsRootless() {
		return errors.New("restoring a container requires root")
	}

	runtime, err := adapter.GetRuntime(getContext(), &c.PodmanCommand)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}
	defer runtime.Shutdown(false)

	options := libpod.ContainerCheckpointOptions{
		Keep:           c.Keep,
		TCPEstablished: c.TcpEstablished,
	}
	return runtime.Restore(c, options)
}
