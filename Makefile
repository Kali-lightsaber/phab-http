OUTPUT=
.PHONY: all

all: build format

build:
	go build -o $(OUTPUT)phab-http phab-http.go

format:
	exit $(shell gofmt -l *.go | wc -l)
