FROM golang AS build
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
COPY . .
RUN apt-get update && apt-get install -y --no-install-recommends xz-utils file && \
  curl -Ls https://github.com/upx/upx/releases/download/v5.0.0/upx-5.0.0-${TARGETARCH}_linux.tar.xz -o - | tar xvJf - -C /tmp && \
  cp /tmp/upx-5.0.0-${TARGETARCH}_linux/upx /usr/local/bin/ && \
  chmod +x /usr/local/bin/upx && \
  apt-get remove -y xz-utils && \
  rm -rf /var/lib/apt/lists/*
RUN make all
ENV CGO_ENABLED=0
RUN echo "Building version: $(git describe --tags --always --dirty)}"
RUN echo "Building commit: $(git rev-parse --short HEAD)"
RUN go build -o /app/kairos-init --ldflags "-w -s -X github.com/kairos-io/kairos-init/pkg/values.version=$(git describe --tags --always --dirty) -X github.com/kairos-io/kairos-init/pkg/values.gitCommit=$(git rev-parse --short HEAD)"


FROM scratch
COPY --from=build /app/kairos-init /kairos-init
ENTRYPOINT ["/kairos-init"]

