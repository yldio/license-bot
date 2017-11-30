FROM golang:1.9.2-alpine

ARG DIR="github.com/yldio/license-bot"
RUN apk add --no-cache --update alpine-sdk

COPY . /go/src/$DIR
RUN cd /go/src/$DIR && make release-binary

FROM alpine:3.4
RUN apk add --update ca-certificates openssl

WORKDIR /go/src/$DIR
COPY --from=0 /go/src/github.com/yldio/license-bot/bin/license-bot /usr/local/bin/license-bot
WORKDIR /

RUN ls -la /usr/local/bin
ENTRYPOINT ["license-bot"]
