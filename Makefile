OUTPUT=
TAG=$(shell git describe --tags 2>/dev/null)
.PHONY: all
ifeq ($(TAG),)
TAG := $(shell echo "dev")
endif

all:
	go build -ldflags "-X main.built=`date +%Y-%m-%dT%H:%M:%S`; -X main.vers=`echo $(TAG)`" -o $(OUTPUT)phab-http phab-http.go
