OUTPUT=bin/
SRC=src/
VERS=
ifeq ($(VERS),)
	VERS=master
endif
export GOPATH := $(PWD)/vendor
.PHONY: all

all: clean build format

clean:
	rm -rf $(OUTPUT)
	mkdir -p $(OUTPUT)

build:
	go build -o $(OUTPUT)phab-http -ldflags '-X main.version=$(VERS)' $(SRC)phab-http.go

format:
	exit $(shell gofmt -l $(SRC)/* | wc -l)

deps:
	git submodule update --init
