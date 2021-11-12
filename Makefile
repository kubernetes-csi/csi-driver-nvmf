# Copyright 2021 The Kubernetes Authors.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#     http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

CMDS=nvmfplugin
REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)

all: build

build-%:
	mkdir -p output
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./output/$* ./cmd/nvmfplugin/$*.go

container-%: build-%
	docker build -t $*:latest -f Dockerfile --label revision=$(REV) .

build: ${CMDS:%=build-%}
container: ${CMDS:%=container-%}
clean:
	-rm -rf output/
