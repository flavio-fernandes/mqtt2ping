# https://github.com/chemidy/smallest-secured-golang-docker-image
ARG  BUILDER_IMAGE=golang:latest
ARG  DISTROLESS_IMAGE=gcr.io/distroless/static
############################
# STEP 1 build executable binary
############################
FROM ${BUILDER_IMAGE} as builder

# Ensure ca-certficates are up to date
RUN update-ca-certificates

RUN mkdir /app
WORKDIR /app

ENV GO111MODULE=on
COPY . .

# Build the static binary
RUN cd cmd/mqtt2ping && \
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
-ldflags='-w -s -extldflags "-static"' -a \
-v -o /go/bin/mqtt2ping .

############################
# STEP 2 build a small image
############################
# using static nonroot image
# user:group is nobody:nobody, uid:gid = 65534:65534
FROM ${DISTROLESS_IMAGE}

# Copy our static executable
COPY --from=builder /go/bin/mqtt2ping /mqtt2ping

ENTRYPOINT ["/mqtt2ping"]
