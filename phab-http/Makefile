OUTPUT=
.PHONY: all

all:
	go build -ldflags "-X main.built=`date +%Y-%m-%dT%H:%M:%S`" -o $(OUTPUT)phab-http phab-http.go
