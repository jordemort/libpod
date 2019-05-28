# Shortcomings of Rootless Podman

The following list categorizes the known issues and irregularities with running Podman as a non-root user.  Although currently functional, there is still a number of work items that are under consideration to be added.  These proposed changes are in varying degrees of design and development.

Contributors are more than welcomed to help with this work.  If you decide to carve off a piece and work on it, please create an issue in [GitHub](https://github.com/containers/libpod/issues), and assign it to yourself.  If you find other unexpected behaviour with rootless Podman and feel it’s warranted, please feel free to update this document.

* Podman can not create containers that bind to ports < 1024.
  * The kernel does not allow processes without CAP_NET_BIND_SERVICE to bind to low ports.
* Lacking “How To” documentation or documentation in general
* If /etc/subuid and /etc/subgid not setup for a user, then podman commands
can easily fail
  * This can be a big issue on machines using Network Based Password information (FreeIPA, Active Directory, LDAP)
  * We are working to get support for NSSWITCH on the /etc/subuid and /etc/subgid files.
* No cgroup Support (hopefully fixed when cgroups V2 happens).
  * Cgroups V1 does not safely support cgroup delegation.
  * Cgroups V2 development for container support is ongoing.
* Can not share container images with CRI-O or other users
* Difficult to use additional stores for sharing content
* Does not work on NFS homedirs
  * NFS enforces file creation on different UIDs on the server side and does not understand User Namespace.
  * When a container root process like YUM attempts to create a file owned by a different UID, NFS Server denies the creation.
* Does not work with homedirs mounted with noexec/nodev
  * User can setup storage to point to other directories they can write to that are not mounted noexec/nodev
* Can not use overlayfs driver, but does support fuse-overlayfs
  * Ubuntu supports non root overlay, but no other Linux distros do.
* Only other supported driver is VFS.
* No KATA Container support
* No CNI Support
  * CNI wants to modify IPTables, plus other network manipulation that I requires CAP_SYS_ADMIN.
  * There is potential we could probably do some sort of blacklisting of the relevant plugins, and add a new plugin for rootless networking - slirp4netns as one example and there may be others
* Cannot use ping
  * [(Can be fixed by setting sysctl on host)](https://github.com/containers/libpod/blob/master/troubleshooting.md#5-rootless-containers-cannot-ping-hosts)
* Requires new shadow-utils (not found in older (RHEL7/Centos7 distros) Should be fixed in RHEL7.7 release
* A few commands do not work.
  * mount/unmount (on fuse-overlay)
     * Only works if you enter the mount namespace with a tool like buildah unshare
  * podman stats (Lack of Cgroup support)
  * Checkpoint and Restore (CRIU requires root)
  * Pause and Unpause (no freezer cgroup)
* Issues with higher UIDs can cause builds to fail
  * If a build is attempting to use a UID that is not mapped into the user namespace mapping for a container, then builds will not be able to put the UID in an image.
