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

%{~ for v in go_installs }
RUN go install ${v}
%{ endfor ~}

FROM --platform=linux/amd64 debian:bookworm

RUN apt-get update && apt-get install -y \
    libc-dev \
    gcc \
    g++ \
    make \
    cmake \
    tzdata && \
    rm -rf /var/lib/apt/lists/*

ENV PATH="/usr/local/bin:/usr/local/go/bin:$PATH"

# node
COPY --from=node /usr/local/bin /usr/local/bin
COPY --from=node /usr/local/lib/node_modules/npm /usr/local/node_modules/npm

# bun
COPY --from=bun /usr/local/bin /usr/local/bin
ENV BUN_INSTALL="/usr/local"

%{~ for v in bun_installs }
RUN bun install -g ${v}
%{ endfor ~}

# uv
COPY --from=uv /usr/local/bin /usr/local/bin
COPY --from=python /usr/local/bin /usr/local/bin
COPY --from=python /usr/local/lib /usr/local/lib
COPY --from=python /usr/local/include /usr/local/include
ENV UV_COMPILE_BYTECODE="1"
ENV UV_INSTALL_DIR="/usr/local/bin"
ENV UV_TOOL_BIN_DIR="/usr/local/bin"

%{~ for v in uv_installs }
RUN uv tool install ${v}
%{ endfor ~}

# go installs
COPY --from=goinstalls /go/bin /usr/local/go/bin
ENV GOROOT="/usr/local/go"
ENV GOBIN="/usr/local/go/bin"

# build file
COPY --from=build /go/src/github.com/miyamo2/slackbot-mcp-host/bin/slackbot-mcp-host /usr/local/go/bin/slackbot-mcp-host

CMD ["/usr/local/go/bin/slackbot-mcp-host"]