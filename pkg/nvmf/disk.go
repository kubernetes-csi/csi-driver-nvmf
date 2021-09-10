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
