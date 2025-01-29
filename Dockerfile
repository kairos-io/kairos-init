FROM golang AS build
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -o /app/kairos-init .


FROM scratch
COPY --from=build /app/kairos-init /kairos-init

