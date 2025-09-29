/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package nvmf

const (
	NVMF_NQN_SIZE = 223
	SYS_NVMF      = "/sys/class/nvme"
	RUN_NVMF      = "/run/nvmf"
)

// Here erron
const (
	ENOENT = 1 /* No such file or directory */
	EINVAL = 2 /* Invalid argument */
)

const (
	DefaultDriverName        = "csi.nvmf.com"
	DefaultDriverServicePort = "12230"
	DefaultDriverVersion     = "v1.0.0"

	DefaultVolumeMapPath = "/var/lib/kubelet/plugins/csi.nvmf.com/volumes"
)

type GlobalConfig struct {
	NVMfVolumeMapDir   string
	DriverName         string
	Region             string
	NodeID             string
	Endpoint           string // CSI endpoint
	Version            string
	IsControllerServer bool
	LogLevel           string
}
