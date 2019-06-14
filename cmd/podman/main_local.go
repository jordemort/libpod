// +build !remoteclient

package main

import (
	"context"
	"log/syslog"
	"os"
	"runtime/pprof"
	"strings"
	"syscall"

	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/cmd/podman/libpodruntime"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/containers/libpod/pkg/tracing"
	"github.com/containers/libpod/pkg/util"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	lsyslog "github.com/sirupsen/logrus/hooks/syslog"
	"github.com/spf13/cobra"
)

const remote = false

func init() {

	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.CGroupManager, "cgroup-manager", "", "Cgroup manager to use (cgroupfs or systemd, default systemd)")
	// -c is deprecated due to conflict with -c on subcommands
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.CpuProfile, "cpu-profile", "", "Path for the cpu profiling results")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.Config, "config", "", "Path of a libpod config file detailing container server configuration options")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.ConmonPath, "conmon", "", "Path of the conmon binary")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.NetworkCmdPath, "network-cmd-path", "", "Path to the command for configuring the network")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.CniConfigDir, "cni-config-dir", "", "Path of the configuration directory for CNI networks")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.DefaultMountsFile, "default-mounts-file", "", "Path to default mounts file")
	rootCmd.PersistentFlags().MarkHidden("defaults-mount-file")
	// Override default --help information of `--help` global flag
	var dummyHelp bool
	rootCmd.PersistentFlags().BoolVar(&dummyHelp, "help", false, "Help for podman")
	rootCmd.PersistentFlags().StringSliceVar(&MainGlobalOpts.HooksDir, "hooks-dir", []string{}, "Set the OCI hooks directory path (may be set multiple times)")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.LogLevel, "log-level", "error", "Log messages above specified level: debug, info, warn, error, fatal or panic")
	rootCmd.PersistentFlags().IntVar(&MainGlobalOpts.MaxWorks, "max-workers", 0, "The maximum number of workers for parallel operations")
	rootCmd.PersistentFlags().MarkHidden("max-workers")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.Namespace, "namespace", "", "Set the libpod namespace, used to create separate views of the containers and pods on the system")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.Root, "root", "", "Path to the root directory in which data, including images, is stored")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.Runroot, "runroot", "", "Path to the 'run directory' where all state information is stored")
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.Runtime, "runtime", "", "Path to the OCI-compatible binary used to run containers, default is /usr/bin/runc")
	// -s is depracated due to conflict with -s on subcommands
	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.StorageDriver, "storage-driver", "", "Select which storage driver is used to manage storage of images and containers (default is overlay)")
	rootCmd.PersistentFlags().StringSliceVar(&MainGlobalOpts.StorageOpts, "storage-opt", []string{}, "Used to pass an option to the storage driver")
	rootCmd.PersistentFlags().BoolVar(&MainGlobalOpts.Syslog, "syslog", false, "Output logging information to syslog as well as the console")

	rootCmd.PersistentFlags().StringVar(&MainGlobalOpts.TmpDir, "tmpdir", "", "Path to the tmp directory")
	rootCmd.PersistentFlags().BoolVar(&MainGlobalOpts.Trace, "trace", false, "Enable opentracing output")
}

func setSyslog() error {
	if MainGlobalOpts.Syslog {
		hook, err := lsyslog.NewSyslogHook("", "", syslog.LOG_INFO, "")
		if err == nil {
			logrus.AddHook(hook)
			return nil
		}
		return err
	}
	return nil
}

func profileOn(cmd *cobra.Command) error {
	if cmd.Flag("cpu-profile").Changed {
		f, err := os.Create(MainGlobalOpts.CpuProfile)
		if err != nil {
			return errors.Wrapf(err, "unable to create cpu profiling file %s",
				MainGlobalOpts.CpuProfile)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}
	}

	if cmd.Flag("trace").Changed {
		var tracer opentracing.Tracer
		tracer, closer = tracing.Init("podman")
		opentracing.SetGlobalTracer(tracer)

		span = tracer.StartSpan("before-context")

		Ctx = opentracing.ContextWithSpan(context.Background(), span)
	}
	return nil
}

func profileOff(cmd *cobra.Command) error {
	if cmd.Flag("cpu-profile").Changed {
		pprof.StopCPUProfile()
	}
	if cmd.Flag("trace").Changed {
		span.Finish()
		closer.Close()
	}
	return nil
}

func setupRootless(cmd *cobra.Command, args []string) error {
	if os.Geteuid() == 0 || cmd == _searchCommand || cmd == _versionCommand || cmd == _mountCommand || cmd == _migrateCommand || strings.HasPrefix(cmd.Use, "help") {
		return nil
	}
	podmanCmd := cliconfig.PodmanCommand{
		cmd,
		args,
		MainGlobalOpts,
		remoteclient,
	}

	pausePidPath, err := util.GetRootlessPauseProcessPidPath()
	if err != nil {
		return errors.Wrapf(err, "could not get pause process pid file path")
	}

	if _, err := os.Stat(pausePidPath); err == nil {
		became, ret, err := rootless.TryJoinFromFilePaths("", false, []string{pausePidPath})
		if err != nil {
			logrus.Errorf("cannot join pause process.  You may need to remove %s and stop all containers", pausePidPath)
			logrus.Errorf("you can use `system migrate` to recreate the pause process")
			logrus.Errorf(err.Error())
			os.Exit(1)
		}
		if became {
			os.Exit(ret)
		}
	}

	// if there is no pid file, try to join existing containers, and create a pause process.

	runtime, err := libpodruntime.GetRuntime(getContext(), &podmanCmd)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}
	defer runtime.Shutdown(false)

	ctrs, err := runtime.GetRunningContainers()
	if err != nil {
		logrus.Errorf(err.Error())
		os.Exit(1)
	}

	paths := []string{}
	for _, ctr := range ctrs {
		paths = append(paths, ctr.Config().ConmonPidFile)
	}

	became, ret, err := rootless.TryJoinFromFilePaths(pausePidPath, true, paths)
	if err != nil {
		logrus.Errorf(err.Error())
		os.Exit(1)
	}
	if became {
		os.Exit(ret)
	}
	return nil
}

func setRLimits() error {
	rlimits := new(syscall.Rlimit)
	rlimits.Cur = 1048576
	rlimits.Max = 1048576
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, rlimits); err != nil {
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, rlimits); err != nil {
			return errors.Wrapf(err, "error getting rlimits")
		}
		rlimits.Cur = rlimits.Max
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, rlimits); err != nil {
			return errors.Wrapf(err, "error setting new rlimits")
		}
	}
	return nil
}

func setUMask() {
	// Be sure we can create directories with 0755 mode.
	syscall.Umask(0022)
}
