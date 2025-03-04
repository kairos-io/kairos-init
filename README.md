kairos-init its an initializer for container images to be Kairosified

You only need to run this once inside a Dockerfile to have a system that has all the necessary tools to run Kairos.

## Usage


Create a Dockerfile with your wanted base image in the FROM, copy the kairos-init binary from the kairos-init image and run it:
```Dockerfile
FROM quay.io/kairos/kairos-init:latest AS kairos-init

FROM ubuntu:20.04
COPY --from=kairos-init /kairos-init /kairos-init
RUN /kairos-init
RUN rm /kairos-init
```

and simply run it, tag it it appropriately:

```bash
docker build -t my-kairosified-image .
[+] Building 106.9s (11/11) FINISHED                                                     docker:default
 => [internal] load build definition from Dockerfile.test                                          0.0s
 => => transferring dockerfile: 194B                                                               0.0s
 => WARN: FromAsCasing: 'as' and 'FROM' keywords' casing do not match (line 1)                     0.0s
 => [internal] load metadata for docker.io/library/ubuntu:20.04                                    0.8s
 => [internal] load metadata for quay.io/kairos/kairos-init:latest                                           0.0s
 => [auth] library/ubuntu:pull token for registry-1.docker.io                                      0.0s
 => [internal] load .dockerignore                                                                  0.0s
 => => transferring context: 2B                                                                    0.0s
 => [kairos-init 1/1] FROM quay.io/kairos/kairos-init:latest                                                 0.0s
 => [stage-1 1/4] FROM docker.io/library/ubuntu:20.04@sha256:8e5c4f0285ecbb4ead070431d29b576a530d  0.0s
 => CACHED [stage-1 2/4] COPY --from=kairos-init /kairos-init /kairos-init                         0.0s
 => [stage-1 3/4] RUN /kairos-init                                                               102.9s
 => [stage-1 4/4] RUN rm /kairos-init                                                              0.2s
 => exporting to image                                                                             2.9s 
 => => exporting layers                                                                            2.9s 
 => => writing image sha256:1789e18e3170974abdbc0cba9071ea25b0251b21c2bf3eb59ad5ff7319ecb11b       0.0s 
 => => naming to docker.io/library/kairos-image     
```


Then you can use [Auroraboot](https://github.com/kairos-io/auroraboot) to transform that image into an ISO, RAW image or as a upgrade source for a running Kairos system.


## Building args

There is several switches that you can use to customize the behavior of kairos-init and obtain the expected artifact:

 - `-f`: set the framework version to use (default: v2.15.3)
 - `-m`: model to build for, like generic or rpi4/rpi3/etc.. (default: generic)
 - `-t`: init the system for Trusted Boot artifact, changes bootloader to systemd. This is only available for the generic model and defaults to using SecureBoot if not enabled.
 - `-v`: variant to build (core or standard for k3s flavor)(default: core)
 - `--fips`: enable FIPS mode (default: false)
 - `--version`: set the Kairos version to use for the built artifact. This is for you to track the version of the image you are building for upgrades and such.
 - `-k`: Kubernetes provider to use, currently supports k3s and k3os (default: k3s)
 - `--k8s-version`: set the Kubernetes version to use for the given provider (default: latest)
- `--stage-extensions`: enable the loading of stage extensions from a dir in the filesystem to extend the default stages with custom logic. See below for more details.

There is also two switches to help you build the image:
 - `-l`: set the log level (default: info). You can choose between info, warn, error, debug for a more verbose output. Remember to use the docker switch `--progress=plain` to see the output correctly.
 - `-s`: set the stage to run (default: all). You can choose between all, install and init to run only a specific stage of the process. Useful if you need to customize the image after the packages are installed but before the system is initialized, like adding modules to initramfs or adding extra packages or scripts.
 - `--validate`: run the validation of the image. This runs a series of tests to validate that the image conforms to the Kairos needs. This has to be run after running the install and init stages.


## Stages

The image conversion is currently split in two different phases:
 - Install: This stage installs all the necessary packages to run Kairos. This includes the kernel, bootloader, framework, etc.
 - Init: This stage initializes the system, like setting up the kernel, configuring the services, generating the initramfs, etc.


## Extending stages with custom actions

This allows to load stage extensions from a dir in the filesystem to expand the default stages with custom logic.

You can enable this feature by using the `--stage-extensions` flag

The structure is as follows:

We got a base dir which is `/etc/kairos-init/stage-extensions` (this is the default, but you can override it using the `KAIROS_INIT_STAGE_EXTENSIONS_DIR` env var)

You can drop your custom [yip files](https://github.com/mudler/yip) and as usual, they will be loaded and executed in lexicographic order.

So for example, if we have:
 - /etc/kairos-init/stage-extensions/10-foo.yaml
 - /etc/kairos-init/stage-extensions/20-bar.yaml
 - /etc/kairos-init/stage-extensions/30-baz.yaml

The files will be loaded in the following order:
 - 10-foo.yaml
 - 20-bar.yaml
 - 30-baz.yaml

The files are loaded using the yip library, so you can use all the features of [yip]((https://github.com/mudler/yip)) to expand the stages.

The current stages available are:
- before-install: Good for adding extra repos and such.
- install: Good for installing packages and such.
- after-install: Do some cleanup of packages, add extra packages, add different kernels and remove the kairos default one, etc.
- before-init: Good for adding some dracut modules for example to be added to the initramfs.
- init: Anything that configures the system, security hardening for example.
- after-init: Good for rebuilding the initramfs, or adding a different initramfs like a kdump one, add grub configs or branding, etc.

So for example, if we were to add an extra repo for zfs and install the package we could do the following:

`/etc/kairos-init/stage-extensions/10-zfs.yaml`
```yaml
stages:
  after-install:
    - files:
        - path: /etc/apt/sources.list.d/zfs.list
          permissions: 0644
          owner: 0
          group: 0
          content: |
            deb http://deb.debian.org/debian bookworm-backports main contrib
            deb-src http://deb.debian.org/debian bookworm-backports main contrib
    - packages:
        install:
          - "zfs-dkms"
          - "zfsutils-linux"
        refresh: true
```

This would run the `before-install` and `install` stages as normal, but then on the `after-install` stage it would add the zfs repo and install the zfs packages.


