# Builds a docker image running Confluent's distribution of Apache Kafka.
# Needs to be linked with a Zookeeper container with alias "zookeeper".
#
# Usage:
#   docker build -t confluent/kafka kafka
#   docker run -d --name kafka --link zookeeper:zookeeper confluent/kafka
FROM confluent/platform
MAINTAINER contact@confluent.io
ENV LOG_DIR "/var/log/kafka"
ENV KAFKA_LOG_DIRS "/var/lib/kafka"
ADD kafka-docker.sh /usr/local/bin/
RUN ["mkdir", "-p", "/var/lib/kafka", "/var/log/kafka", "/etc/security"]
RUN ["chown", "-R", "confluent:confluent", "/var/lib/kafka", "/var/log/kafka", "/etc/kafka/server.properties"]
RUN ["chmod", "+x", "/usr/local/bin/kafka-docker.sh"]
VOLUME ["/var/lib/kafka", "/var/log/kafka", "/etc/security"]
#TODO Update the ports that are exposed.
#TODO Add support to expose JMX
EXPOSE 9092
USER confluent
ENTRYPOINT ["/usr/local/bin/kafka-docker.sh"]

# build stage
FROM golang as builder
ARG MODULE
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
