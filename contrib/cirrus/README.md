![PODMAN logo](../../logo/podman-logo-source.svg)

# Cirrus-CI

Similar to other integrated github CI/CD services, Cirrus utilizes a simple
YAML-based configuration/description file: ``.cirrus.yml``.  Ref: https://cirrus-ci.org/


## Workflow

All tasks execute in parallel, unless there are conditions or dependencies
which alter this behavior.  Within each task, each script executes in sequence,
so long as any previous script exited successfully.  The overall state of each
task (pass or fail) is set based on the exit status of the last script to execute.


### ``gating`` Task

***N/B: Steps below are performed by automation***

1. Launch a purpose-built container in Cirrus's community cluster.
   For container image details, please see
   [the contributors guide](https://github.com/containers/libpod/blob/master/CONTRIBUTING.md#go-format-and-lint).

3. ``validate``: Perform standard `make validate` source verification,
   Should run for less than a minute or two.

4. ``lint``: Execute regular `make lint` to check for any code cruft.
   Should also run for less than a few minutes.

5. ``vendor``: runs `make vendor` followed by `./hack/tree_status.sh` to check
   whether the git tree is clean. The reasoning for that is to make sure that
   the vendor.conf, the code and the vendored packages in ./vendor are in sync
   at all times.

### ``meta`` Task

***N/B: Steps below are performed by automation***

1. Launch a container built from definition in ``./contrib/imgts``.

2. Update VM Image metadata to help track usage across all automation.

4. Always exits successfully unless there's a major problem.


### ``testing`` Task

***N/B: Steps below are performed by automation***

1. After `gating` passes, spin up one VM per
   `matrix: image_name` item. Once accessible, ``ssh``
   into each VM as the `root` user.

2. ``setup_environment.sh``: Configure root's `.bash_profile`
    for all subsequent scripts (each run in a new shell).  Any
    distribution-specific environment variables are also defined
    here.  For example, setting tags/flags to use compiling.

5. ``integration_test.sh``: Execute integration-testing.  This is
   much more involved, and relies on access to external
   resources like container images and code from other repositories.
   Total execution time is capped at 2-hours (includes all the above)
   but this script normally completes in less than an hour.

### ``special_testing`` Task

This task exercises podman under specialized environments or conditions.
The specific differences from the ``testing`` task depend upon the
contents of the ``$SPECIALMODE`` environment variable.

| Value     | Meaning                                                               |
| rootless  | Setup a regular user to build/run integration tests.                  |
| in_podman | Setup a container image, build/run integration tests inside container |

***N/B: Steps below are performed by automation***

1. After `gating` passes, spin up one VM per
   `matrix: image_name` item.

2. ``setup_environment.sh``: Mostly the same as
   in ``testing`` task, then specialized depending on ``$SPECIALMODE``.

3. Which tests and how they execute depends on ``$SPECIALMODE``.


### ``optional_testing`` Task

***N/B: Steps below are performed by automation***

1. Optionally executes in parallel with ``testing``.  Requires
    **prior** to job-start, the magic string ``***CIRRUS: SYSTEM TEST***``
   is found in the pull-request *description*.  The *description* is the first
   text-box under the main *summary* line in the github WebUI.

2. ``setup_environment.sh``: Same as for other tasks.

3. ``system_test.sh``: Build both dependencies and libpod, install them,
   then execute `make localsystem` from the repository root.


### ``test_build_cache_images_task`` Task

Modifying the contents of cache-images is tested by making changes to
one or more of the ``./contrib/cirrus/packer/*_setup.sh`` files.  Then
in the PR description, add the magic string:  ``***CIRRUS: TEST IMAGES***``

***N/B: Steps below are performed by automation***

1. ``setup_environment.sh``: Same as for other tasks.

2. ``build_vm_images.sh``: Utilize [the packer tool](http://packer.io/docs/)
   to produce new VM images.  Create a new VM from each base-image, connect
   to them with ``ssh``, and perform the steps as defined by the
   ``$PACKER_BASE/libpod_images.yml`` file:

    1. On a base-image VM, as root, copy the current state of the repository
       into ``/tmp/libpod``.
    2. Execute distribution-specific scripts to prepare the image for
       use.  For example, ``fedora_setup.sh``.
    3. If successful, shut down each VM and record the names, and dates
       into a json manifest file.
    4. Move the manifest file, into a google storage bucket object.
       This is a retained as a secondary method for tracking/auditing
       creation of VM images, should it ever be needed.

### ``verify_test_built_images`` Task

Only runs following successful ``test_build_cache_images_task`` task.  Uses
images following the standard naming format; ***however, only runs a limited
sub-set of automated tests***.  Validating newly built images fully, requires
updating ``.cirrus.yml``.

***Manual Steps:***  Assuming `verify_test_built_images` passes, then
you'll find the new image names displayed at the end of the
`test_build_cache_images_task` in the `build_vm_images` output.
For example:


```
...cut...
==> Builds finished. The artifacts of successful builds are:
--> ubuntu-18: A disk image was created: ubuntu-18-libpod-5699523102900224
--> ubuntu-18:
--> fedora-29: A disk image was created: fedora-29-libpod-5699523102900224
--> fedora-29:
--> fedora-28: A disk image was created: fedora-28-libpod-5699523102900224
```

Now edit `.cirrus.yml`, updating the `*_IMAGE_NAME` lines to reflect the
images from above:


```yaml
env:
    ...cut...
    ####
    #### Cache-image names to test with
    ###
    FEDORA_CACHE_IMAGE_NAME: "fedora-29-libpod-5699523102900224"
    PRIOR_FEDORA_CACHE_IMAGE_NAME: "fedora-28-libpod-5699523102900224"
    UBUNTU_CACHE_IMAGE_NAME: "ubuntu-18-libpod-5699523102900224"
    ...cut...
```

***NOTE:*** If re-using the same PR with new images in `.cirrus.yml`,
take care to also *update the PR description* to remove
the magic ``***CIRRUS: TEST IMAGES***`` string.  Keeping it and
`--force` pushing would needlessly cause Cirrus-CI to build
and test images again.


### ``build_cache_images`` Task  *(Deprecated)*

Exactly the same as ``test_build_cache_images_task`` task, but only runs on
the master branch.  Requires a magic string to be in the `HEAD`
commit message: ``***CIRRUS: BUILD IMAGES***``

When successful, the manifest file along with all VM disks, are moved
into a dedicated google storage bucket, separate from the one used by
`test_build_cache_images_task`.  These may be used to create new cache-images for
PR testing by manually importing them as described above.


### Base-images

Base-images are VM disk-images specially prepared for executing as GCE VMs.
In particular, they run services on startup similar in purpose/function
as the standard 'cloud-init' services.

*  The google services are required for full support of ssh-key management
   and GCE OAuth capabilities.  Google provides native images in GCE
   with services pre-installed, for many platforms. For example,
   RHEL, CentOS, and Ubuntu.

*  Google does ***not*** provide any images for Fedora or Fedora Atomic
   Host (as of 11/2018), nor do they provide a base-image prepared to
   run packer for creating other images in the ``build_vm_images`` Task
   (above).

*  Base images do not need to be produced often, but doing so completely
   manually would be time-consuming and error-prone.  Therefor a special
   semi-automatic *Makefile* target is provided to assist with producing
   all the base-images: ``libpod_base_images``

To produce new base-images, including an `image-builder-image` (used by
the ``cache_images`` Task) some input parameters are required:

* ``GCP_PROJECT_ID``: The complete GCP project ID string e.g. foobar-12345
  identifying where the images will be stored.

* ``GOOGLE_APPLICATION_CREDENTIALS``: A *JSON* file containing
  credentials for a GCE service account.  This can be [a service
  account](https://cloud.google.com/docs/authentication/production#obtaining_and_providing_service_account_credentials_manually)
  or [end-user
  credentials](https://cloud.google.com/docs/authentication/end-user#creating_your_client_credentials)

*  Optionally, CSV's may be specified to ``PACKER_BUILDS``
   to limit the base-images produced.  For example,
   ``PACKER_BUILDS=fedora,image-builder-image``.

If there is an existing 'image-builder-image' within GCE, it may be utilized
to produce base-images (in addition to cache-images).  However it must be
created with support for nested-virtualization, and with elevated cloud
privileges (to access GCE, from within the GCE VM).  For example:

```
$ alias pgcloud='sudo podman run -it --rm -e AS_ID=$UID
    -e AS_USER=$USER -v $HOME:$HOME:z quay.io/cevich/gcloud_centos:latest'

$ URL=https://www.googleapis.com/auth
$ SCOPES=$URL/userinfo.email,$URL/compute,$URL/devstorage.full_control

# The --min-cpu-platform is critical for nested-virt.
$ pgcloud compute instances create $USER-making-images \
    --image-family image-builder-image \
    --boot-disk-size "200GB" \
    --min-cpu-platform "Intel Haswell" \
    --machine-type n1-standard-2 \
    --scopes $SCOPES
```

Alternatively, if there is no image-builder-image available yet, a bare-metal
CentOS 7 machine with network access to GCE is required.  Software dependencies
can be obtained from the ``packer/image-builder-image_base_setup.sh`` script.

In both cases, the following can be used to setup and build base-images.

```
$ IP_ADDRESS=1.2.3.4  # EXTERNAL_IP from command output above
$ rsync -av $PWD centos@$IP_ADDRESS:.
$ scp $GOOGLE_APPLICATION_CREDENTIALS centos@$IP_ADDRESS:.
$ ssh centos@$IP_ADDRESS
...
```

When ready, change to the ``packer`` sub-directory, and build the images:

```
$ cd libpod/contrib/cirrus/packer
$ make libpod_base_images GCP_PROJECT_ID=<VALUE> \
    GOOGLE_APPLICATION_CREDENTIALS=<VALUE> \
    PACKER_BUILDS=<OPTIONAL>
```

Assuming this is successful (hence the semi-automatic part), packer will
produce a ``packer-manifest.json`` output file.  This contains the base-image
names suitable for updating in ``.cirrus.yml``, `env` keys ``*_BASE_IMAGE``.

On failure, it should be possible to determine the problem from the packer
output.  Sometimes that means setting `PACKER_LOG=1` and troubleshooting
the nested virt calls.  It's also possible to observe the (nested) qemu-kvm
console output.  Simply set the ``TTYDEV`` parameter, for example:

```
$ make libpod_base_images ... TTYDEV=$(tty)
  ...
```
