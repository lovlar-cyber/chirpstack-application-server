# build stage
FROM golang as builder
ARG MODULE
# confluent platform
RUN wget -qO - http://packages.confluent.io/deb/3.1/archive.key | sudo apt-key add -
RUN add-apt-repository "deb [arch=amd64] http://packages.confluent.io/deb/3.1 stable main"
RUN apt-get update && apt-get install confluent-platform-2.11
# librdkafka Build from source
RUN git clone https://github.com/edenhill/librdkafka.git
WORKDIR librdkafka
RUN ./configure --prefix /usr
RUN make
RUN make install
# Build go binary
WORKDIR /app/
COPY ./src/${MODULE} ${MODULE}
ENV GO111MODULE=on
WORKDIR /app/${MODULE}
RUN ls
RUN go mod download
RUN go build -o main -tags dynamic
RUN ls
# final stage
FROM ubuntu
ARG MODULE
COPY --from=builder /usr/lib/pkgconfig /usr/lib/pkgconfig
COPY --from=builder /usr/lib/librdkafka* /usr/lib/
COPY --from=builder /app/${MODULE}/* /${MODULE}/
WORKDIR /${MODULE}
CMD ["./main"]


# Original below
FROM golang:1.14-alpine AS development

ENV PROJECT_PATH=/chirpstack-application-server
ENV PATH=$PATH:$PROJECT_PATH/build
ENV CGO_ENABLED=0
ENV GO_EXTRA_BUILD_ARGS="-a -installsuffix cgo "

RUN apk add --no-cache ca-certificates make git bash alpine-sdk nodejs nodejs-npm build-base

RUN mkdir -p $PROJECT_PATH
COPY . $PROJECT_PATH
WORKDIR $PROJECT_PATH


RUN make dev-requirements ui-requirements
RUN make

FROM alpine:3.11.2 AS production

RUN apk --no-cache add ca-certificates
COPY --from=development /chirpstack-application-server/build/chirpstack-application-server /usr/bin/chirpstack-application-server
ENTRYPOINT ["/usr/bin/chirpstack-application-server"]
