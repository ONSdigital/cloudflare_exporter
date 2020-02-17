ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
ADD cloudflare_exporter /bin/cloudflare_exporter

EXPOSE     9199
USER       nobody
ENTRYPOINT ["/bin/cloudflare_exporter"]
