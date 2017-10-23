.PHONY: all controller executor clean test

all: controller executor

controller:
	go build -o _output/bin/node-fencing-controller cmd/node-fencing-controller.go

executor:
	go build -o _output/bin/node-fencing-executor cmd/node-fencing-executor.go

clean:
	-rm -rf _output

test:
	go test `go list ./... | grep -v 'vendor'`
