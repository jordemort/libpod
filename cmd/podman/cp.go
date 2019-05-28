package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/buildah/pkg/chrootuser"
	"github.com/containers/buildah/util"
	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/cmd/podman/libpodruntime"
	"github.com/containers/libpod/libpod"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/chrootarchive"
	"github.com/containers/storage/pkg/idtools"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	cpCommand cliconfig.CpValues

	cpDescription = `Command copies the contents of SRC_PATH to the DEST_PATH.

  You can copy from the container's file system to the local machine or the reverse, from the local filesystem to the container. If "-" is specified for either the SRC_PATH or DEST_PATH, you can also stream a tar archive from STDIN or to STDOUT. The CONTAINER can be a running or stopped container.  The SRC_PATH or DEST_PATH can be a file or directory.
`
	_cpCommand = &cobra.Command{
		Use:   "cp [flags] SRC_PATH DEST_PATH",
		Short: "Copy files/folders between a container and the local filesystem",
		Long:  cpDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			cpCommand.InputArgs = args
			cpCommand.GlobalFlags = MainGlobalOpts
			cpCommand.Remote = remoteclient
			return cpCmd(&cpCommand)
		},
		Example: "[CONTAINER:]SRC_PATH [CONTAINER:]DEST_PATH",
	}
)

func init() {
	cpCommand.Command = _cpCommand
	flags := cpCommand.Flags()
	flags.BoolVar(&cpCommand.Extract, "extract", false, "Extract the tar file into the destination directory.")
	cpCommand.SetHelpTemplate(HelpTemplate())
	cpCommand.SetUsageTemplate(UsageTemplate())
	rootCmd.AddCommand(cpCommand.Command)
}

func cpCmd(c *cliconfig.CpValues) error {
	args := c.InputArgs
	if len(args) != 2 {
		return errors.Errorf("you must provide a source path and a destination path")
	}

	runtime, err := libpodruntime.GetRuntime(getContext(), &c.PodmanCommand)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}
	defer runtime.Shutdown(false)

	extract := c.Flag("extract").Changed
	return copyBetweenHostAndContainer(runtime, args[0], args[1], extract)
}

func copyBetweenHostAndContainer(runtime *libpod.Runtime, src string, dest string, extract bool) error {

	srcCtr, srcPath := parsePath(runtime, src)
	destCtr, destPath := parsePath(runtime, dest)

	if (srcCtr == nil && destCtr == nil) || (srcCtr != nil && destCtr != nil) {
		return errors.Errorf("invalid arguments %s, %s you must use just one container", src, dest)
	}

	if len(srcPath) == 0 || len(destPath) == 0 {
		return errors.Errorf("invalid arguments %s, %s you must specify paths", src, dest)
	}
	ctr := srcCtr
	isFromHostToCtr := (ctr == nil)
	if isFromHostToCtr {
		ctr = destCtr
	}

	mountPoint, err := ctr.Mount()
	if err != nil {
		return err
	}
	defer ctr.Unmount(false)
	user, err := getUser(mountPoint, ctr.User())
	if err != nil {
		return err
	}
	idMappingOpts, err := ctr.IDMappings()
	if err != nil {
		return errors.Wrapf(err, "error getting IDMappingOptions")
	}
	containerOwner := idtools.IDPair{UID: int(user.UID), GID: int(user.GID)}
	hostUID, hostGID, err := util.GetHostIDs(convertIDMap(idMappingOpts.UIDMap), convertIDMap(idMappingOpts.GIDMap), user.UID, user.GID)
	if err != nil {
		return err
	}

	hostOwner := idtools.IDPair{UID: int(hostUID), GID: int(hostGID)}

	var glob []string
	if isFromHostToCtr {
		if filepath.IsAbs(destPath) {
			destPath = filepath.Join(mountPoint, destPath)

		} else {
			if err = idtools.MkdirAllAndChownNew(filepath.Join(mountPoint, ctr.WorkingDir()), 0755, hostOwner); err != nil {
				return errors.Wrapf(err, "error creating directory %q", destPath)
			}
			destPath = filepath.Join(mountPoint, ctr.WorkingDir(), destPath)
		}
	} else {
		if filepath.IsAbs(srcPath) {
			srcPath = filepath.Join(mountPoint, srcPath)
		} else {
			srcPath = filepath.Join(mountPoint, ctr.WorkingDir(), srcPath)
		}
	}
	glob, err = filepath.Glob(srcPath)
	if err != nil {
		return errors.Wrapf(err, "invalid glob %q", srcPath)
	}
	if len(glob) == 0 {
		glob = append(glob, srcPath)
	}
	if !filepath.IsAbs(destPath) {
		dir, err := os.Getwd()
		if err != nil {
			return errors.Wrapf(err, "err getting current working directory")
		}
		destPath = filepath.Join(dir, destPath)
	}

	var lastError error
	for _, src := range glob {
		if src == "-" {
			src = os.Stdin.Name()
			extract = true
		}
		err := copy(src, destPath, dest, idMappingOpts, &containerOwner, extract, isFromHostToCtr)
		if lastError != nil {
			logrus.Error(lastError)
		}
		lastError = err
	}
	return lastError
}

func getUser(mountPoint string, userspec string) (specs.User, error) {
	uid, gid, err := chrootuser.GetUser(mountPoint, userspec)
	u := specs.User{
		UID:      uid,
		GID:      gid,
		Username: userspec,
	}
	if !strings.Contains(userspec, ":") {
		groups, err2 := chrootuser.GetAdditionalGroupsForUser(mountPoint, uint64(u.UID))
		if err2 != nil {
			if errors.Cause(err2) != chrootuser.ErrNoSuchUser && err == nil {
				err = err2
			}
		} else {
			u.AdditionalGids = groups
		}

	}
	return u, err
}

func parsePath(runtime *libpod.Runtime, path string) (*libpod.Container, string) {
	pathArr := strings.SplitN(path, ":", 2)
	if len(pathArr) == 2 {
		ctr, err := runtime.LookupContainer(pathArr[0])
		if err == nil {
			return ctr, pathArr[1]
		}
	}
	return nil, path
}

func getPathInfo(path string) (string, os.FileInfo, error) {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, errors.Wrapf(err, "error evaluating symlinks %q", path)
	}
	srcfi, err := os.Stat(path)
	if err != nil {
		return "", nil, errors.Wrapf(err, "error reading path %q", path)
	}
	return path, srcfi, nil
}

func copy(src, destPath, dest string, idMappingOpts storage.IDMappingOptions, chownOpts *idtools.IDPair, extract, isFromHostToCtr bool) error {
	srcPath, err := filepath.EvalSymlinks(src)
	if err != nil {
		return errors.Wrapf(err, "error evaluating symlinks %q", srcPath)
	}

	srcPath, srcfi, err := getPathInfo(srcPath)
	if err != nil {
		return err
	}

	filename := filepath.Base(destPath)
	if filename == "-" && !isFromHostToCtr {
		err := streamFileToStdout(srcPath, srcfi)
		if err != nil {
			return errors.Wrapf(err, "error streaming source file %s to Stdout", srcPath)
		}
		return nil
	}

	destdir := destPath
	if !srcfi.IsDir() && !strings.HasSuffix(dest, string(os.PathSeparator)) {
		destdir = filepath.Dir(destPath)
	}
	_, err = os.Stat(destdir)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "error checking directory %q", destdir)
	}
	destDirIsExist := (err == nil)
	if err = os.MkdirAll(destdir, 0755); err != nil {
		return errors.Wrapf(err, "error creating directory %q", destdir)
	}

	// return functions for copying items
	copyFileWithTar := chrootarchive.CopyFileWithTarAndChown(chownOpts, digest.Canonical.Digester().Hash(), idMappingOpts.UIDMap, idMappingOpts.GIDMap)
	copyWithTar := chrootarchive.CopyWithTarAndChown(chownOpts, digest.Canonical.Digester().Hash(), idMappingOpts.UIDMap, idMappingOpts.GIDMap)
	untarPath := chrootarchive.UntarPathAndChown(chownOpts, digest.Canonical.Digester().Hash(), idMappingOpts.UIDMap, idMappingOpts.GIDMap)

	if srcfi.IsDir() {
		logrus.Debugf("copying %q to %q", srcPath+string(os.PathSeparator)+"*", dest+string(os.PathSeparator)+"*")
		if destDirIsExist && !strings.HasSuffix(src, fmt.Sprintf("%s.", string(os.PathSeparator))) {
			destPath = filepath.Join(destPath, filepath.Base(srcPath))
		}
		if err = copyWithTar(srcPath, destPath); err != nil {
			return errors.Wrapf(err, "error copying %q to %q", srcPath, dest)
		}
		return nil
	}
	if !archive.IsArchivePath(srcPath) {
		// This srcPath is a file, and either it's not an
		// archive, or we don't care whether or not it's an
		// archive.
		destfi, err := os.Stat(destPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrapf(err, "failed to get stat of dest path %s", destPath)
			}
		}
		if destfi != nil && destfi.IsDir() {
			destPath = filepath.Join(destPath, filepath.Base(srcPath))
		}
	}

	if extract {
		// We're extracting an archive into the destination directory.
		logrus.Debugf("extracting contents of %q into %q", srcPath, destPath)
		if err = untarPath(srcPath, destPath); err != nil {
			return errors.Wrapf(err, "error extracting %q into %q", srcPath, destPath)
		}
		return nil
	}
	// Copy the file, preserving attributes.
	logrus.Debugf("copying %q to %q", srcPath, destPath)
	if err = copyFileWithTar(srcPath, destPath); err != nil {
		return errors.Wrapf(err, "error copying %q to %q", srcPath, destPath)
	}
	return nil
}

func convertIDMap(idMaps []idtools.IDMap) (convertedIDMap []specs.LinuxIDMapping) {
	for _, idmap := range idMaps {
		tempIDMap := specs.LinuxIDMapping{
			ContainerID: uint32(idmap.ContainerID),
			HostID:      uint32(idmap.HostID),
			Size:        uint32(idmap.Size),
		}
		convertedIDMap = append(convertedIDMap, tempIDMap)
	}
	return convertedIDMap
}

func streamFileToStdout(srcPath string, srcfi os.FileInfo) error {
	if srcfi.IsDir() {
		tw := tar.NewWriter(os.Stdout)
		err := filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.Mode().IsRegular() || path == srcPath {
				return err
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}

			if err = tw.WriteHeader(hdr); err != nil {
				return err
			}
			fh, err := os.Open(path)
			if err != nil {
				return err
			}
			defer fh.Close()

			_, err = io.Copy(tw, fh)
			return err
		})
		if err != nil {
			return errors.Wrapf(err, "error streaming directory %s to Stdout", srcPath)
		}
		return nil
	}

	file, err := os.Open(srcPath)
	if err != nil {
		return errors.Wrapf(err, "error opening file %s", srcPath)
	}
	defer file.Close()
	if !archive.IsArchivePath(srcPath) {
		tw := tar.NewWriter(os.Stdout)
		hdr, err := tar.FileInfoHeader(srcfi, "")
		if err != nil {
			return err
		}
		err = tw.WriteHeader(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, file)
		if err != nil {
			return errors.Wrapf(err, "error streaming archive %s to Stdout", srcPath)
		}
		return nil
	}

	_, err = io.Copy(os.Stdout, file)
	if err != nil {
		return errors.Wrapf(err, "error streaming file to Stdout")
	}
	return nil
}
