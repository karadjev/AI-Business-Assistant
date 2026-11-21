# Local archives must be supplied by the separately approved preparation phase.
# No package downloads in this Dockerfile. Retain archives, checksums and image ID.
FROM golang:1.26.2-alpine3.23
COPY apks/ /toolchain-apks/
RUN cd /toolchain-apks && sha256sum -c SHA256SUMS && apk add --no-network /toolchain-apks/*.apk && gcc --version && go version
ENV CGO_ENABLED=1 CC=gcc CXX=g++ GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOTELEMETRY=off
