package main

import (
	"context"
	"io"
	"os"

	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/libpod"
	_ "github.com/containers/libpod/pkg/hooks/0.1.0"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/containers/libpod/version"
	"github.com/containers/storage/pkg/reexec"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// This is populated by the Makefile from the VERSION file
// in the repository
var (
	exitCode = 125
	Ctx      context.Context
	span     opentracing.Span
	closer   io.Closer
)

// Commands that the remote and local client have
// implemented.
var mainCommands = []*cobra.Command{
	_attachCommand,
	_buildCommand,
	_diffCommand,
	_createCommand,
	_eventsCommand,
	_exportCommand,
	_generateCommand,
	_historyCommand,
	&_imagesCommand,
	_importCommand,
	_infoCommand,
	_initCommand,
	&_inspectCommand,
	_killCommand,
	_loadCommand,
	_logsCommand,
	_pauseCommand,
	podCommand.Command,
	_portCommand,
	&_psCommand,
	_pullCommand,
	_pushCommand,
	_restartCommand,
	_rmCommand,
	&_rmiCommand,
	_runCommand,
	_saveCommand,
	_stopCommand,
	_tagCommand,
	_topCommand,
	_umountCommand,
	_unpauseCommand,
	_versionCommand,
	_waitCommand,
	imageCommand.Command,
	_startCommand,
	systemCommand.Command,
}

var rootCmd = &cobra.Command{
	Use:  "podman",
	Long: "manage pods and images",
	RunE: commandRunE(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return before(cmd, args)
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return after(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

var MainGlobalOpts cliconfig.MainFlags

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.TraverseChildren = true
	rootCmd.Version = version.Version
	// Override default --help information of `--version` global flag
	var dummyVersion bool
	rootCmd.PersistentFlags().BoolVar(&dummyVersion, "version", false, "Version for podman")
	rootCmd.AddCommand(mainCommands...)
	rootCmd.AddCommand(getMainCommands()...)
}

func initConfig() {
	//	we can do more stuff in here.
}

func before(cmd *cobra.Command, args []string) error {
	if err := libpod.SetXdgRuntimeDir(""); err != nil {
		logrus.Errorf(err.Error())
		os.Exit(1)
	}
	if err := setupRootless(cmd, args); err != nil {
		return err
	}

	//	Set log level; if not log-level is provided, default to error
	logLevel := MainGlobalOpts.LogLevel
	if logLevel == "" {
		logLevel = "error"
	}
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(level)

	if err := setRLimits(); err != nil {
		return err
	}
	if rootless.IsRootless() {
		logrus.Info("running as rootless")
	}
	setUMask()
	return profileOn(cmd)
}

func after(cmd *cobra.Command, args []string) error {
	return profileOff(cmd)
}

func main() {
	//debug := false
	//cpuProfile := false

	if reexec.Init() {
		return
	}
	if err := rootCmd.Execute(); err != nil {
		outputError(err)
	} else {
		// The exitCode modified from 125, indicates an application
		// running inside of a container failed, as opposed to the
		// podman command failed.  Must exit with that exit code
		// otherwise command exited correctly.
		if exitCode == 125 {
			exitCode = 0
		}

	}

	// Check if /etc/containers/registries.conf exists when running in
	// in a local environment.
	CheckForRegistries()
	os.Exit(exitCode)
}
