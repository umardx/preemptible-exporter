# build stage
FROM golang:1.15-alpine3.12

ADD . /go/src/preemptible-exporter
WORKDIR /go/src/preemptible-exporter
RUN go build -o /bin/preemptible-exporter .

FROM alpine:3.12.0
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
COPY --from=0 /bin/preemptible-exporter /bin
ENTRYPOINT ["/bin/preemptible-exporter"]
