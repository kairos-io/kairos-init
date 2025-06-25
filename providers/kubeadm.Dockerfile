ARG VERSION=1.0.0
FROM wcrum/kairos-init:providers AS kairos-init
FROM golang:1.24 AS provider-builder

WORKDIR /
RUN git clone https://github.com/kairos-io/provider-kubeadm.git
WORKDIR /provider-kubeadm
ENV GO_LDFLAGS=" -X github.com/kairos-io/kairos/provider-kubeadm/version.Version=${VERSION} -w -s"
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o agent-provider-kubeadm ./main.go

FROM ubuntu:24.04
ARG VERSION=1.0.0
COPY --from=provider-builder /provider-kubeadm/agent-provider-kubeadm /system/provider
COPY --from=kairos-init /kairos-init /kairos-init
RUN /kairos-init --version "${VERSION}" --kubernetes-provider=kubeadm --k8sversion=v1.30.0
RUN rm /kairos-init
