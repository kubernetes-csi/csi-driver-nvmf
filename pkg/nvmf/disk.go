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
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

type nvmfDiskInfo struct {
	VolId      string
	Nqn        string
	Addr       string
	Port       string
	DeviceUUID string
	Transport  string
}

func getNVMfInfo(req *csi.NodePublishVolumeRequest) (*nvmfDiskInfo, error) {
	volId := req.GetVolumeId()
	volOpts := req.GetVolumeContext()

	targetTrAddr := volOpts["targetTrAddr"]
	targetTrPort := volOpts["targetTrPort"]
	targetTrType := volOpts["targetTrType"]
	deviceUUID := volOpts["deviceUUID"]
	nqn := volOpts["nqn"]

	if targetTrAddr == "" || nqn == "" || targetTrPort == "" || targetTrType == "" {
		return nil, fmt.Errorf("Some Nvme target info is missing, volID: %s ", volId)
	}

	return &nvmfDiskInfo{
		VolId:      volId,
		Addr:       targetTrAddr,
		Port:       targetTrPort,
		Nqn:        nqn,
		DeviceUUID: deviceUUID,
		Transport:  targetTrType,
	}, nil
}
