OUTPUT=
.PHONY: all

all:
	go build -o $(OUTPUT)phab-http phab-http.go
