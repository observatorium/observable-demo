FROM golang:1.13 as builder

ADD . /opt
WORKDIR /opt

RUN git update-index --refresh; make build

FROM prom/prometheus:v2.15.2 as runner

COPY --from=builder /opt/loadbalancer /bin/loadbalancer
