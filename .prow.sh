#! /bin/bash

# Copyright 2022 The Kubernetes Authors.
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

# A Prow job can override these defaults, but this shouldn't be necessary.

# disable e2e tests for now
CSI_PROW_E2E_REPO=none
CSI_PROW_GO_VERSION_BUILD=1.19
# Only these tests make sense for unit test
: ${CSI_PROW_TESTS:="unit"}

. release-tools/prow.sh

main
