% podman-container-restore(1)

## NAME
podman\-container\-restore - Restores one or more running containers

## SYNOPSIS
**podman container restore** [*options*] *container* ...

## DESCRIPTION
Restores a container from a checkpoint. You may use container IDs or names as input.

## OPTIONS
**--keep**, **-k**

Keep all temporary log and statistics files created by CRIU during
checkpointing as well as restoring. These files are not deleted if restoring
fails for further debugging. If restoring succeeds these files are
theoretically not needed, but if these files are needed Podman can keep the
files for further analysis. This includes the checkpoint directory with all
files created during checkpointing. The size required by the checkpoint
directory is roughly the same as the amount of memory required by the
processes in the checkpointed container.

Without the **-k**, **--keep** option the checkpoint will be consumed and cannot be used
again.

**--all, -a**

Restore all checkpointed containers.

**--latest, -l**

Instead of providing the container name or ID, restore the last created container.

The latest option is not supported on the remote client.

**--tcp-established**

Restore a container with established TCP connections. If the checkpoint image
contains established TCP connections, this option is required during restore.
If the checkpoint image does not contain established TCP connections this
option is ignored. Defaults to not restoring containers with established TCP
connections.

**--import, -i**

Import a checkpoint tar.gz file, which was exported by Podman. This can be used
to import a checkpointed container from another host. It is not necessary to specify
a container when restoring from an exported checkpoint.

**--name, -n**

This is only available in combination with **--import, -i**. If a container is restored
from a checkpoint tar.gz file it is possible to rename it with **--name, -n**. This
way it is possible to restore a container from a checkpoint multiple times with different
names.

If the **--name, -n** option is used, Podman will not attempt to assign the same IP
address to the container it was using before checkpointing as each IP address can only
be used once and the restored container will have another IP address. This also means
that **--name, -n** cannot be used in combination with **--tcp-established**.

## EXAMPLE

podman container restore mywebserver

podman container restore 860a4b23

## SEE ALSO
podman(1), podman-container-checkpoint(1)

## HISTORY
September 2018, Originally compiled by Adrian Reber <areber@redhat.com>
