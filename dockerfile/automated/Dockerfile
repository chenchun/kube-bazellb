FROM haproxy:1.8.2-alpine

# install golang 1.9.7 https://github.com/docker-library/golang/blob/fd09bcfba2f2ec52318834df502b1a1907fa04f5/1.9/alpine3.7/Dockerfile
ENV GOLANG_VERSION 1.9.7
RUN set -eux; \
	apk add --no-cache --virtual .build-deps \
		bash \
		gcc \
		musl-dev \
		openssl \
		git \
		go
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"
WORKDIR $GOPATH

RUN go get github.com/chenchun/kube-bmlb/cmd/bmlb
EXPOSE 80 8081 9010
CMD ["kube-bmlb", "--logtostderr"]