FROM golang AS build
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN echo "Building version: $(git describe --tags --always --dirty)}"
RUN echo "Building commit: $(git rev-parse --short HEAD)"
RUN go build -o /app/kairos-init --ldflags "-w -s -X github.com/kairos-io/kairos-init/pkg/values.version=$(git describe --tags --always --dirty) -X github.com/kairos-io/kairos-init/pkg/values.gitCommit=$(git rev-parse --short HEAD)"


FROM scratch
COPY --from=build /app/kairos-init /kairos-init
ENTRYPOINT ["/kairos-init"]

