# syntax=docker/dockerfile:1.10
FROM --platform=linux/amd64 node:lts-bookworm as node
FROM --platform=linux/amd64 oven/bun:1.2.9-debian as bun
FROM --platform=linux/amd64 ghcr.io/astral-sh/uv:0.6.14-python3.13-bookworm as uv
FROM --platform=linux/amd64 python:3.13.0-bookworm as python

FROM --platform=linux/amd64 golang:1.24.2-bookworm as build

WORKDIR /go/src/github.com/miyamo2/slackbot-mcp-host

COPY . .

RUN go build -ldflags="-s -w" -trimpath -o ./bin/slackbot-mcp-host ./cmd/main.go

FROM --platform=linux/amd64 golang:1.24.2-bookworm as goinstalls

%{ for v in go_installs }
RUN go install ${v}
%{ endfor ~}

FROM --platform=linux/amd64 golang:1.24.2-bookworm

RUN apt-get update && apt-get install -y \
    libc-dev \
    gcc \
    g++ \
    make \
    cmake \
    tzdata && \
    rm -rf /var/lib/apt/lists/*

# node
COPY --from=node /usr/local/bin /usr/local/bin
COPY --from=node /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm

# bun
COPY --from=bun /usr/local/bin /usr/local/bin

# uv
COPY --from=uv /usr/local/bin /usr/local/bin
COPY --from=python /usr/local/bin /usr/local/bin
COPY --from=python /usr/local/lib /usr/local/lib
COPY --from=python /usr/local/include /usr/local/include

# go installs
COPY --from=goinstalls /go/bin /go/bin

# build file
COPY --from=build /go/src/github.com/miyamo2/slackbot-mcp-host/bin/slackbot-mcp-host /app/slackbot-mcp-host

CMD ["/app/slackbot-mcp-host"]