ARG GO_VERSION=1.27.0

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY api/ ./api/
COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/louder-operator ./cmd/operator

FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/louder-operator /louder-operator

USER 65532:65532
EXPOSE 8080 8081
ENTRYPOINT ["/louder-operator"]
