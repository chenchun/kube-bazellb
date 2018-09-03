.PHONY: default
default: build

GIT_COMMIT=git-$(shell git rev-parse --short HEAD)
GIT_REPO=$(shell git config --get remote.origin.url)
GOOS=linux
GOARCH=amd64
PACKAGE=github.com/chenchun/kube-bmlb

.PHONY: build
build:
	@mkdir -p bin
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build \
	  -ldflags "-s -w -X version.COMMIT=$(GIT_COMMIT) -X version.REPO=$(GIT_REPO)" \
	  -o bin/bmlb \
	  $(PACKAGE)/cmd/bmlb

.PHONY: image
image:
	docker build --network=host -t chenchun/kube-bmlb -f dockerfile/bmlb/Dockerfile .
