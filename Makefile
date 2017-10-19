.PHONY: all controller clean test

all: controller

controller:
	go build -o _output/bin/node-fencing-controller cmd/node-fencing.go

clean:
	-rm -rf _output

test:
	go test `go list ./... | grep -v 'vendor'`
