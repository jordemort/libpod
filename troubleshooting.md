![PODMAN logo](logo/podman-logo-source.svg)

# Troubleshooting

## A list of common issues and solutions for Podman

---
### 1) Variety of issues - Validate Version

A large number of issues reported against Podman are often found to already be fixed
in more current versions of the project.  Before reporting an issue, please verify the
version you are running with `podman version` and compare it to the latest release
documented on the top of Podman's [README.md](README.md).

If they differ, please update your version of PODMAN to the latest possible
and retry your command before reporting the issue.

---
### 2) No such image or Bare keys cannot contain ':'

When doing a `podman pull` or `podman build` command and a "common" image cannot be pulled,
it is likely that the `/etc/containers/registries.conf` file is either not installed or possibly
misconfigured.

#### Symptom

```console
$ sudo podman build -f Dockerfile
STEP 1: FROM alpine
error building: error creating build container: no such image "alpine" in registry: image not known
```

or

```console
$ sudo podman pull fedora
error pulling image "fedora": unable to pull fedora: error getting default registries to try: Near line 9 (last key parsed ''): Bare keys cannot contain ':'.
```

#### Solution

  * Verify that the `/etc/containers/registries.conf` file exists.  If not, verify that the containers-common package is installed.
  * Verify that the entries in the `[registries.search]` section of the /etc/containers/registries.conf file are valid and reachable.
    *  i.e. `registries = ['registry.fedoraproject.org', 'quay.io', 'registry.access.redhat.com']`

---
### 3) http: server gave HTTP response to HTTPS client

When doing a Podman command such as `build`, `commit`, `pull`, or `push` to a registry,
tls verification is turned on by default.  If authentication is not used with
those commands, this error can occur.

#### Symptom

```console
$ sudo podman push alpine docker://localhost:5000/myalpine:latest
Getting image source signatures
Get https://localhost:5000/v2/: http: server gave HTTP response to HTTPS client
```

#### Solution

By default tls verification is turned on when communicating to registries from
Podman.  If the registry does not require authentication the Podman commands
such as `build`, `commit`, `pull` and `push` will fail unless tls verification is turned
off using the `--tls-verify` option.  **NOTE:** It is not at all recommended to
communicate with a registry and not use tls verification.

  * Turn off tls verification by passing false to the tls-verification option.
  * I.e. `podman push --tls-verify=false alpine docker://localhost:5000/myalpine:latest`

---
### 4) Rootless: could not get runtime - database configuration mismatch

In Podman release 0.11.1, a default path for rootless containers was changed,
potentially causing rootless Podman to be unable to function. The new default
path is not a problem for new installations, but existing installations will
need to work around it with the following fix.

#### Symptom

```console
$ podman info
could not get runtime: database run root /run/user/1000/run does not match our run root /run/user/1000: database configuration mismatch
```

#### Solution

This problem has been fixed in Podman release 0.12.1 and it is recommended
to upgrade to that version.  If that is not possible use the following procedure.

To work around the new default path, we can manually set the path Podman is
expecting in a configuration file.

First, we need to make a new local configuration file for rootless Podman.
* `mkdir -p ~/.config/containers`
* `cp /usr/share/containers/libpod.conf ~/.config/containers`

Next, edit the new local configuration file
(`~/.config/containers/libpod.conf`) with your favorite editor. Comment out the
line starting with `cgroup_manager` by adding a `#` character at the beginning
of the line, and change the path in the line starting with `tmp_dir` to point to
the first path in the error message Podman gave (in this case,
`/run/user/1000/run`).

---
### 5) rootless containers cannot ping hosts

When using the ping command from a non-root container, the command may
fail because of a lack of privileges.

#### Symptom

```console
$ podman run --rm fedora ping -W10 -c1 redhat.com
PING redhat.com (209.132.183.105): 56 data bytes

--- redhat.com ping statistics ---
1 packets transmitted, 0 packets received, 100% packet loss
```

#### Solution

It is most likely necessary to enable unprivileged pings on the host.
Be sure the UID of the user is part of the range in the
`/proc/sys/net/ipv4/ping_group_range` file.

To change its value you can use something like: `sysctl -w
"net.ipv4.ping_group_range=0 2000000"`.

To make the change persistent, you'll need to add a file in
`/etc/sysctl.d` that contains `net.ipv4.ping_group_range=0 $MAX_UID`.

---
### 6) Build hangs when the Dockerfile contains the useradd command

When the Dockerfile contains a command like `RUN useradd -u 99999000 -g users newuser` the build can hang.

#### Symptom

If you are using a useradd command within a Dockerfile with a large UID/GID, it will create a large sparse file `/var/log/lastlog`.  This can cause the build to hang forever.  Go language does not support sparse files correctly, which can lead to some huge files being created in your container image.

#### Solution

If the entry in the Dockerfile looked like: RUN useradd -u 99999000 -g users newuser then add the `--log-no-init` parameter to change it to: `RUN useradd --log-no-init -u 99999000 -g users newuser`. This option tells useradd to stop creating the lastlog file.

### 7) Permission denied when running Podman commands

When rootless podman attempts to execute a container on a non exec home directory a permission error will be raised.

#### Symptom

If you are running podman or buildah on a home directory that is mounted noexec,
then they will fail. With a message like:

```
podman run centos:7
standard_init_linux.go:203: exec user process caused "permission denied"
```

#### Solution

Since the administrator of the system setup your home directory to be noexec, you will not be allowed to execute containers from storage in your home directory. It is possible to work around this by manually specifying a container storage path that is not on a noexec mount. Simply copy the file /etc/containers/storage.conf to ~/.config/containers/ (creating the directory if necessary). Specify a graphroot directory which is not on a noexec mount point and to which you have read/write privileges.  You will need to modify other fields to writable directories as well.

For example

```
cat ~/.config/containers/storage.conf
[storage]
  driver = "overlay"
  runroot = "/run/user/1000"
  graphroot = "/execdir/myuser/storage"
  [storage.options]
    mount_program = "/bin/fuse-overlayfs"
```

### 8) Permission denied when running systemd within a Podman container

When running systemd as PID 1 inside of a container on an SELinux
separated machine, it needs to write to the cgroup file system.

#### Symptom

Systemd gets permission denied when attempting to write to the cgroup file
system, and AVC messages start to show up in the audit.log file or journal on
the system.

#### Solution

SELinux provides a boolean `container_manage_cgroup`, which allows container
processes to write to the cgroup file system. Turn on this boolean, on SELinux separated systems, to allow systemd to run properly in the container.

`setsebool -P container_manage_cgroup true`

### 9) Newuidmap missing when running rootless Podman commands

Rootless podman requires the newuidmap and newgidmap programs to be installed.

#### Symptom

If you are running podman or buildah as a not root user, you get an error complaining about
a missing newuidmap executable.

```
podman run -ti fedora sh
cannot find newuidmap: exec: "newuidmap": executable file not found in $PATH
```

#### Solution

Install a version of shadow-utils that includes these executables.  Note RHEL7 and Centos 7 will not have support for this until RHEL7.7 is released.

### 10) rootless setup user: invalid argument

Rootless podman requires the user running it to have a range of UIDs listed in /etc/subuid and /etc/subgid.

#### Symptom

An user, either via --user or through the default configured for the image, is not mapped inside the namespace.

```
podman run --rm -ti --user 1000000 alpine echo hi
Error: container create failed: container_linux.go:344: starting container process caused "setup user: invalid argument"
```

#### Solution

Update the /etc/subuid and /etc/subgid with fields for users that look like:

```
cat /etc/subuid
johndoe:100000:65536
test:165536:65536
```

The format of this file is USERNAME:UID:RANGE

* username as listed in /etc/passwd or getpwent.
* The initial uid allocated for the user.
* The size of the range of UIDs allocated for the user.

This means johndoe is allocated UIDS 100000-165535 as well as his standard UID in the
/etc/passwd file.

You should ensure that each user has a unique range of uids, because overlapping UIDs,
would potentially allow one user to attack another user.

You could also use the usermod program to assign UIDs to a user.

If you update either the /etc/subuid or /etc/subgid file, you need to
stop all running containers and kill the pause process.  This is done
automatically by the `system migrate` command, which can also be used
to stop all the containers and kill the pause process.

```
usermod --add-subuids 200000-201000 --add-subgids 200000-201000 johndoe
grep johndoe /etc/subuid /etc/subgid
/etc/subuid:johndoe:200000:1001
/etc/subgid:johndoe:200000:1001
```

### 11) Changing the location of the Graphroot leads to permission denied

When I change the graphroot storage location in storage.conf, the next time I
run podman I get an error like:

```
# podman run -p 5000:5000 -it centos bash

bash: error while loading shared libraries: /lib64/libc.so.6: cannot apply additional memory protection after relocation: Permission denied
```

For example, the admin sets up a spare disk to be mounted at `/src/containers`,
and points storage.conf at this directory.


#### Symptom

SELinux blocks containers from using random locations for overlay storage.
These directories need to be labeled with the same labels as if the content was
under /var/lib/containers/storage.

#### Solution

Tell SELinux about the new containers storage by setting up an equivalence record.
This tells SELinux to label content under the new path, as if it was stored
under `/var/lib/containers/storage`.

```
semanage fcontext -a -e /var/lib/containers /srv/containers
restorecon -R -v /src/containers
```

The semanage command above tells SELinux to setup the default labeling of
`/srv/containers` to match `/var/lib/containers`.  The `restorecon` command
tells SELinux to apply the labels to the actual content.

Now all new content created in these directories will automatically be created
with the correct label.

### 12) Running Podman inside a container causes container crashes and inconsistent states

Running Podman in a container and forwarding some, but not all, of the required host directories can cause inconsistent container behavior.

#### Symptom

After creating a container with Podman's storage directories mounted in from the host and running Podman inside a container, all containers show their state as "configured" or "created", even if they were running or stopped.

#### Solution

When running Podman inside a container, it is recommended to mount at a minimum `/var/lib/containers/storage/` as a volume.
Typically, you will not mount in the host version of the directory, but if you wish to share containers with the host, you can do so.
If you do mount in the host's `/var/lib/containers/storage`, however, you must also mount in the host's `/var/run/libpod` and `/var/run/containers/storage` directories.
Not doing this will cause Podman in the container to detect that temporary files have been cleared, leading it to assume a system restart has taken place.
This can cause Podman to reset container states and lose track of running containers.

For running containers on the host from inside a container, we also recommend the [Podman remote client](remote_client.md), which only requires a single socket to be mounted into the container.
