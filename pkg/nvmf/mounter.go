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

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

type nvmfDiskMounter struct {
	*nvmfDiskInfo
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         exec.Interface
	targetPath   string
	connector    *Connector
}

type nvmfDiskUnMounter struct {
	mounter mount.Interface
	exec    exec.Interface
}

func getNVMfDiskMounter(nvmfInfo *nvmfDiskInfo, req *csi.NodePublishVolumeRequest) *nvmfDiskMounter {
	readOnly := req.GetReadonly()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()

	return &nvmfDiskMounter{
		nvmfDiskInfo : nvmfInfo,
		readOnly: readOnly,
		fsType: fsType,
		mountOptions: mountOptions,
		mounter:    &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()},
		exec:       exec.New(),
		targetPath: req.GetTargetPath(),
		connector:  getConnector(nvmfInfo),
	}
}

func getNVMfDiskUnMounter(req *csi.NodeUnpublishVolumeRequest) *nvmfDiskUnMounter {
	return &nvmfDiskUnMounter{
		mounter: mount.New(""),
		exec:    exec.New(),
	}
}
