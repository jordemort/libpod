package main

import (
	"fmt"

	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/pkg/adapter"
	"github.com/containers/libpod/pkg/systemdgen"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	containerSystemdCommand     cliconfig.GenerateSystemdValues
	containerSystemdDescription = `Command generates a systemd unit file for a Podman container
  `
	_containerSystemdCommand = &cobra.Command{
		Use:   "systemd [flags] CONTAINER | POD",
		Short: "Generate a systemd unit file for a Podman container",
		Long:  containerSystemdDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			containerSystemdCommand.InputArgs = args
			containerSystemdCommand.GlobalFlags = MainGlobalOpts
			containerSystemdCommand.Remote = remoteclient
			return generateSystemdCmd(&containerSystemdCommand)
		},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 || len(args) < 1 {
				return errors.New("provide only one container name or ID")
			}
			return nil
		},
		Example: `podman generate kube ctrID
`,
	}
)

func init() {
	containerSystemdCommand.Command = _containerSystemdCommand
	containerSystemdCommand.SetHelpTemplate(HelpTemplate())
	containerSystemdCommand.SetUsageTemplate(UsageTemplate())
	flags := containerSystemdCommand.Flags()
	flags.BoolVarP(&containerSystemdCommand.Name, "name", "n", false, "use the container name instead of ID")
	flags.IntVarP(&containerSystemdCommand.StopTimeout, "timeout", "t", -1, "stop timeout override")
	flags.StringVar(&containerSystemdCommand.RestartPolicy, "restart-policy", "on-failure", "applicable systemd restart-policy")
}

func generateSystemdCmd(c *cliconfig.GenerateSystemdValues) error {
	runtime, err := adapter.GetRuntime(getContext(), &c.PodmanCommand)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}
	defer runtime.Shutdown(false)

	// User input stop timeout must be 0 or greater
	if c.Flag("timeout").Changed && c.StopTimeout < 0 {
		return errors.New("timeout value must be 0 or greater")
	}
	// Make sure the input restart policy is valid
	if err := systemdgen.ValidateRestartPolicy(c.RestartPolicy); err != nil {
		return err
	}

	unit, err := runtime.GenerateSystemd(c)
	if err != nil {
		return err
	}
	fmt.Println(unit)
	return nil
}
