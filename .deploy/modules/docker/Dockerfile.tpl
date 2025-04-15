# syntax=docker/dockerfile:1.10
FROM --platform=linux/amd64 golang:1.24.2-bookworm as build

WORKDIR /go/src/github.com/miyamo2/slackbot-mcp-host

COPY . .

RUN go build -ldflags="-s -w" -trimpath -o ./bin/slackbot-mcp-host ./cmd/main.go

FROM --platform=linux/amd64 golang:1.24.2-bookworm as goinstalls

%{ for v in go_installs }
RUN go install ${v}
%{ endfor ~}

FROM --platform=linux/amd64 oven/bun:latest as bun

FROM --platform=linux/amd64 ghcr.io/astral-sh/uv:latest as uv

FROM --platform=linux/amd64 golang:1.24.2-bookworm

COPY --from=bun /usr/local/bin /usr/local/bin

COPY --from=uv /uv /uvx /usr/local/bin/

ENV PATH $PATH:$HOME/.local/bin:/usr/local/bin:~/.local/bin:/usr/bin:/root/.local/bin:~/.bun/bin

COPY --from=goinstalls /go/bin /go/bin
COPY --from=build /go/src/github.com/miyamo2/slackbot-mcp-host/bin/slackbot-mcp-host /app/slackbot-mcp-host

CMD ["/app/slackbot-mcp-host"]