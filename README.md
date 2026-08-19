> ## This repository has been archived pending migration into kairos-io/kairos
>
> kairos-init is being absorbed into the
> [kairos-io/kairos](https://github.com/kairos-io/kairos) monorepo as
> part of the plan tracked in
> [kairos-io/kairos#4301](https://github.com/kairos-io/kairos/issues/4301).
>
> Every existing container tag (`quay.io/kairos/kairos-init:vX.Y.Z`)
> stays published and pullable, so any Dockerfile that pins a specific
> version keeps working. Every existing Go module tag also remains
> resolvable via the Go module proxy.
>
> New development happens at
> [github.com/kairos-io/kairos/kairos-init](https://github.com/kairos-io/kairos/tree/master/kairos-init).
> If that link returns 404, the subtree import is not yet complete;
> see the tracking issue above for the current state and timeline.
>
> **To pick up newer kairos-init code (Go module consumers),** update
> your imports:
>
> ```
> find . -type f -name '*.go' -exec sed -i \
>   's|github.com/kairos-io/kairos-init|github.com/kairos-io/kairos/kairos-init|g' {} +
> go mod tidy
> go get github.com/kairos-io/kairos@<latest>
> ```
>
> **Container consumers do not need to change anything.** The
> `quay.io/kairos/kairos-init` image will continue to publish new
> tags out of the monorepo's release pipeline; the image name and
> registry stay the same.
>
> **On backports to older lines.** The default answer is: consume the
> latest release. We will only unarchive this repository for fixes we
> judge important enough (typically security fixes or breakage with no
> reasonable workaround). Convenience backports and feature backports
> do not qualify. To request one, open an issue on
> [kairos-io/kairos](https://github.com/kairos-io/kairos/issues)
> describing the kairos-init version, the fix, and why moving to the
> latest release is not viable. If we agree the fix meets the bar, we
> will unarchive temporarily to land it and cut a new patch tag.

---

# kairos-init

kairos-init is an initializer for container images to be Kairosified.

You only need to run this once inside a Dockerfile to have a system that has all the necessary tools to run Kairos.

## Quick example

Create a Dockerfile with your desired base image, mount the kairos-init binary from the kairos-init image and run it:

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

## NVIDIA / Jetson

### Jetson AGX Thor QSPI firmware

Thor boards will not boot if the QSPI boot firmware version does not correspond to the L4T
version in the image. Kairos images pin L4T `39.2`.

At install time, `after-install-chroot` runs `/usr/sbin/kairos-jetson-qspi-update` on boards
whose devicetree SoC compatible string is `nvidia,tegra264` (uniform across every Thor
variant: AGX, IGX and devkit). It compares the board's firmware version (from the UEFI ESRT)
against the image's `nvidia-l4t-bootloader` version and:

- stages a UEFI capsule update when the image is newer — applied by UEFI on the next boot;
- does nothing when they match;
- **aborts the install** when the board firmware is newer than the image, because UEFI
  capsule update cannot downgrade firmware. Use a Kairos image matching the board, or
  reflash the board;
- **aborts the install** when the board is below L4T 38.0.0, which needs a USB host flash.

Override the L4T version with the `L4T_VERSION` environment variable.

See [kairos-io/kairos#4228](https://github.com/kairos-io/kairos/issues/4228).

## Documentation

For full documentation — including all available flags, configuration options, examples, extending stages, building for Trusted Boot, RHEL images, and more — please refer to the official Kairos documentation:

**[https://kairos.io/docs/reference/kairos-factory/](https://kairos.io/docs/reference/kairos-factory/)**
