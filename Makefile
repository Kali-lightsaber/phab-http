OUTPUT=
BUILD=$(shell date +%Y-%m-%dT%H:%M:%S)
.PHONY: all

all:
	go build -ldflags "-X main.build=`echo '$(BUILD)'`" -o $(OUTPUT)phab-http phab-http.go
