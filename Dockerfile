FROM golang:1.21

RUN mkdir /go/src/interop-ocp-watcher-bot
WORKDIR /go/src/interop-ocp-watcher-bot

COPY src/* .

RUN go mod download && go install interop-ocp-watcher-bot.go
