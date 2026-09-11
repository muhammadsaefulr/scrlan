FROM golang:1.26.1-bookworm AS build

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends libpcap-dev \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/scrlan ./cmd/worker

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libcap2-bin libpcap0.8 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --create-home --uid 10001 app

COPY --from=build /out/scrlan /usr/local/bin/scrlan
RUN setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/scrlan

USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/scrlan"]
