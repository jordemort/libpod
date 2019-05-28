% podman-container-rm(1)

## NAME
podman\-container\-rm (podman\-rm) - Remove one or more containers

## SYNOPSIS
**podman container rm** [*options*] *container*

**podman rm** [*options*] *container*

## DESCRIPTION
**podman rm** will remove one or more containers from the host.  The container name or ID can be used.  This does not remove images.  Running containers will not be removed without the `-f` option

## OPTIONS

**--all, -a**

Remove all containers.  Can be used in conjunction with -f as well.

**--force, -f**

Force the removal of running and paused containers.  Forcing a containers removal also
removes containers from container storage even if the container is not known to podman.
Containers could have been created by a different container engine.

**--latest, -l**

Instead of providing the container name or ID, use the last created container. If you use methods other than Podman
to run containers such as CRI-O, the last started container could be from either of those methods.

The latest option is not supported on the remote client.

**--volumes, -v**

Remove the volumes associated with the container.

## EXAMPLE
Remove a container by its name *mywebserver*
```
podman rm mywebserver
```
Remove several containers by name and container id.
```
podman rm mywebserver myflaskserver 860a4b23
```

Forcibly remove a container by container ID.
```
podman rm -f 860a4b23
```

Remove all containers regardless of its run state.
```
podman rm -f -a
```

Forcibly remove the latest container created.
```
podman rm -f --latest
```

## Exit Status
**_0_** if all specified containers removed
**_1_** if one of the specified containers did not exist, and no other failures
**_125_** if command fails for a reason other then an container did not exist

## SEE ALSO
podman(1), podman-image-rm(1)

## HISTORY
August 2017, Originally compiled by Ryan Cole <rycole@redhat.com>
