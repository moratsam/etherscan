########################################################
# STEP 1 use a temporary image to build a static binary
########################################################
FROM golang:1.17 AS builder

# Pull build dependencies
WORKDIR $GOPATH/src/github.com/moratsam/etherscan
COPY . .
RUN make deps

# Build static image.
RUN GIT_SHA=$(git rev-parse --short HEAD) && \
    CGO_ENABLED=0 GOARCH=amd64 GOOS=linux \
    go build -a \
    -ldflags "-extldflags '-static' -w -s -X main.appSha=$GIT_SHA" \
    -o /go/bin/etherscan-frontend \
	 github.com/moratsam/etherscan/depl/microservices/frontend

########################################################
# STEP 2 create alpine image with trusted certs
########################################################

FROM alpine:3.10
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
COPY --from=builder /go/bin/etherscan-frontend /go/bin/etherscan-frontend

ENTRYPOINT ["/go/bin/etherscan-frontend"]
