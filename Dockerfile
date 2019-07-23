# Build the manager binary
FROM golang:1.12.7-alpine as builder

# Copy in the go src
WORKDIR /go/src/github.com/packethost/cluster-api-provider-packet
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/
COPY go.* ./
COPY tools.go ./


# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -mod=vendor -a -o manager github.com/packethost/cluster-api-provider-packet/cmd/manager

# Copy the controller-manager into a thin image
FROM alpine:3.10
WORKDIR /
RUN apk --update add ca-certificates
COPY --from=builder /go/src/github.com/packethost/cluster-api-provider-packet/manager .
ENTRYPOINT ["/manager"]
