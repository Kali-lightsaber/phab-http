OUTPUT=$(PWD)/bin/
.PHONY: all

all: clean
	make -C phab-http/ OUTPUT=$(OUTPUT)

clean:
	mkdir -p $(OUTPUT)
	rm -f $(OUTPUT)/*
