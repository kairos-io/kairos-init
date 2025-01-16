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

There is also two switches to help you build the image:
 - `-d`: set the log level (default: info). You can choose between info, warn, error, debug for a more verbose output. Remember to use the docker switch `--progress=plain` to see the output correctly.
 - `-s`: set the stage to run (default: all). You can choose between all, install and init to run only a specific stage of the process. Useful if you need to customize the image after the packages are installed but before the system is initialized, like adding modules to initramfs or adding extra packages or scripts.


## Stages

The image conversion is currently split in two different phases:
 - Install: This stage installs all the necessary packages to run Kairos. This includes the kernel, bootloader, framework, etc.
 - Init: This stage initializes the system, like setting up the kernel, configuring the services, generating the initramfs, etc.

