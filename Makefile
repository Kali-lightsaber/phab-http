OUTPUT=bin/
SRC=src/
.PHONY: all

all: clean build format

clean:
	rm -rf $(OUTPUT)
	mkdir -p $(OUTPUT)

build:
	go build -o $(OUTPUT)phab-http $(SRC)phab-http.go

format:
	exit $(shell gofmt -l $(SRC)/* | wc -l)

deps:
	git submodule update --init
