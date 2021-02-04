
FROM golang:1.14-alpine AS development

ENV PROJECT_PATH=/chirpstack-application-server
ENV PATH=$PATH:$PROJECT_PATH/build
ENV CGO_ENABLED=0
ENV GO_EXTRA_BUILD_ARGS="-a -installsuffix cgo -tags dynamic"

RUN apk add --no-cache ca-certificates make git bash alpine-sdk nodejs nodejs-npm librdkafka-dev build-base

RUN mkdir -p $PROJECT_PATH
COPY . $PROJECT_PATH
WORKDIR $PROJECT_PATH

RUN make dev-requirements ui-requirements
RUN make

FROM alpine:3.11.2 AS production

RUN apk --no-cache add ca-certificates
COPY --from=development /chirpstack-application-server/build/chirpstack-application-server /usr/bin/chirpstack-application-server
ENTRYPOINT ["/usr/bin/chirpstack-application-server"]
