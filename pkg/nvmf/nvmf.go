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
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog/v2"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

// NVMe-oF parameter keys
const (
	paramAddr = "targetTrAddr" // Target address parameter
	paramPort = "targetTrPort" // Target port parameter
	paramType = "targetTrType" // Transport type parameter
)

type nvmfDiskInfo struct {
	VolName    string
	Nqn        string `json:"subnqn"`
	Addr       string `json:"traddr"`
	Port       string `json:"trsvcid"`
	DeviceUUID string
	Transport  string `json:"trtype"`
}

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

func getNVMfDiskInfo(req *csi.NodePublishVolumeRequest) (*nvmfDiskInfo, error) {
	volName := req.GetVolumeId()

	volOpts := req.GetVolumeContext()
	targetTrAddr := volOpts[paramAddr]
	targetTrPort := volOpts[paramPort]
	targetTrType := volOpts[paramType]
	deviceUUID := volOpts["deviceUUID"]
	nqn := volOpts["nqn"]

	if targetTrAddr == "" || nqn == "" || targetTrPort == "" || targetTrType == "" || deviceUUID == "" {
		return nil, fmt.Errorf("some nvme target info is missing, volID: %s ", volName)
	}

	return &nvmfDiskInfo{
		VolName:    volName,
		Addr:       targetTrAddr,
		Port:       targetTrPort,
		Nqn:        nqn,
		DeviceUUID: deviceUUID,
		Transport:  targetTrType,
	}, nil
}

func getNVMfDiskMounter(nvmfInfo *nvmfDiskInfo, req *csi.NodePublishVolumeRequest) *nvmfDiskMounter {
	readOnly := req.GetReadonly()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()

	return &nvmfDiskMounter{
		nvmfDiskInfo: nvmfInfo,
		readOnly:     readOnly,
		fsType:       fsType,
		mountOptions: mountOptions,
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()},
		exec:         exec.New(),
		targetPath:   req.GetTargetPath(),
		connector:    getNvmfConnector(nvmfInfo, req.GetTargetPath()),
	}
}

func getNVMfDiskUnMounter(req *csi.NodeUnpublishVolumeRequest) *nvmfDiskUnMounter {
	return &nvmfDiskUnMounter{
		mounter: mount.New(""),
		exec:    exec.New(),
	}
}

func AttachDisk(req *csi.NodePublishVolumeRequest, nm nvmfDiskMounter) (string, error) {
	if nm.connector == nil {
		return "", fmt.Errorf("connector is nil")
	}

	// connect nvmf target disk
	devicePath, err := nm.connector.Connect()
	if err != nil {
		klog.Errorf("AttachDisk: VolumeID %s failed to connect, Error: %v", req.VolumeId, err)
		return "", err
	}
	if devicePath == "" {
		klog.Errorf("AttachDisk: VolumeId %s return nil devicePath", req.VolumeId)
		return "", fmt.Errorf("VolumeId %s return nil devicePath", req.VolumeId)
	}
	klog.Infof("AttachDisk: Volume %s successful connected, Device：%s", req.VolumeId, devicePath)

	mntPath := nm.targetPath
	klog.Infof("AttachDisk: MntPath: %s", mntPath)
	notMounted, err := nm.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("Heuristic determination of mount point failed:%v", err)
	}
	if !notMounted {
		klog.Infof("AttachDisk: VolumeID: %s, Path: %s is already mounted, device: %s", req.VolumeId, nm.targetPath, nm.DeviceUUID)
		return "", nil
	}

	// pre to mount
	if err := os.MkdirAll(mntPath, 0750); err != nil {
		klog.Errorf("AttachDisk: failed to mkdir %s, error", mntPath)
		return "", err
	}

	err = persistConnectorFile(nm.connector, mntPath+".json")
	if err != nil {
		klog.Errorf("AttachDisk: failed to persist connection info: %v", err)
		klog.Errorf("AttachDisk: disconnecting volume and failing the publish request because persistence file are required for unpublish volume")
		return "", fmt.Errorf("unable to create persistence file for connection")
	}

	// Tips: use k8s mounter to mount fs and only support "ext4"
	var options []string
	options = append(options, nm.mountOptions...)
	err = nm.mounter.FormatAndMount(devicePath, mntPath, nm.fsType, options)
	if err != nil {
		klog.Errorf("AttachDisk: failed to mount Device %s to %s with options: %v", devicePath, mntPath, options)
		nm.connector.Disconnect()
		removeConnectorFile(mntPath)
		return "", fmt.Errorf("failed to mount Device %s to %s with options: %v", devicePath, mntPath, options)
	}

	klog.Infof("AttachDisk: Successfully Mount Device %s to %s with options: %v", devicePath, mntPath, options)
	return devicePath, nil
}

func DetachDisk(volumeID string, num *nvmfDiskUnMounter, targetPath string) error {
	if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExists {
		klog.Warningf("Warning: Unmount skipped because path does not exist: %v", targetPath)
		return nil
	}
	if err := num.mounter.Unmount(targetPath); err != nil {
		klog.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}

	connector, err := GetConnectorFromFile(targetPath + ".json")
	if err != nil {
		klog.Errorf("DetachDisk: failed to get connector from path %s Error: %v", targetPath, err)
		return err
	}
	err = connector.Disconnect()
	if err != nil {
		klog.Errorf("DetachDisk: VolumeID: %s failed to disconnect, Error: %v", volumeID, err)
		return err
	}
	removeConnectorFile(targetPath)
	return nil
}
