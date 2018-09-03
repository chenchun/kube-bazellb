FROM haproxy:1.8.2-alpine

RUN set -eux; \
	apk add --no-cache --virtual .build-deps \
		bash \
		gcc \
		musl-dev \
		openssl \
		git \
		curl \
		ipvsadm \
		iptables \
		ipset

COPY bin/bmlb /bin
EXPOSE 80 9010
CMD ["bmlb", "--logtostderr"]