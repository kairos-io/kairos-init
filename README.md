# kairos-init

kairos-init is an initializer for container images to be Kairosified.

You only need to run this once inside a Dockerfile to have a system that has all the necessary tools to run Kairos.

## Quick example

Create a Dockerfile with your desired base image, copy the kairos-init binary from the kairos-init image and run it:

```Dockerfile
FROM quay.io/kairos/kairos-init:latest AS kairos-init

FROM ubuntu:24.04
ARG VERSION=1.0.0
RUN --mount=type=bind,from=kairos-init,src=/kairos-init,dst=/kairos-init /kairos-init --version "${VERSION}"
```

Then build it:

```bash
docker build -t my-kairosified-image .
```

You can then use [AuroraBoot](https://github.com/kairos-io/auroraboot) to transform that image into an ISO, RAW image, or use it as an upgrade source for a running Kairos system.

## Documentation

For full documentation — including all available flags, configuration options, examples, extending stages, building for Trusted Boot, RHEL images, and more — please refer to the official Kairos documentation:

**[https://kairos.io/docs/reference/kairos-factory/](https://kairos.io/docs/reference/kairos-factory/)**
