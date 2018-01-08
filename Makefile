# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ifeq ($(REGISTRY),)
	REGISTRY = quay.io/bronhaim/
endif

ifeq ($(VERSION),)
	VERSION = latest
endif

IMAGE_CONTROLLER = $(REGISTRY)standalone-fence-controller:$(VERSION)
MUTABLE_IMAGE_CONTROLLER = $(REGISTRY)standalone-fence-controller:latest

.PHONY: all controller clean test

all: controller

controller:
	go build -i -o standalone-controller/_output/bin/node-fencing-controller cmd/node-fencing-controller.go

clean:
	-rm -rf _output

container: controller
	docker build -t $(MUTABLE_IMAGE_CONTROLLER) standalone-controller
	docker tag $(MUTABLE_IMAGE_CONTROLLER) $(IMAGE_CONTROLLER)

test:
	go test `go list ./... | grep -v 'vendor'`
