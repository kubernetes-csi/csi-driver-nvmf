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
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog/v2"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

// NVMe-oF parameter keys
const (
	paramAddr     = "targetTrAddr"     // Target address parameter
	paramPort     = "targetTrPort"     // Target port parameter
	paramType     = "targetTrType"     // Transport type parameter
	paramEndpoint = "targetTrEndpoint" // Target endpoints parameter
)

type nvmfDiskInfo struct {
	VolName   string
	Nqn       string `json:"subnqn"`
	Addr      string `json:"traddr"`
	Port      string `json:"trsvcid"`
	Transport string `json:"trtype"`
	Endpoints []string
}

type nvmfDiskMounter struct {
	*nvmfDiskInfo
	isBlock      bool
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

// getNVMfDiskInfo extracts NVMf disk information from the provided parameters
func getNVMfDiskInfo(volID string, params map[string]string) (*nvmfDiskInfo, error) {
	if params == nil {
		return nil, fmt.Errorf("discovery parameters are nil")
	}

	targetTrType := params[paramType]
	targetTrEndpoints := params[paramEndpoint]
	nqn := volID

	if nqn == "" || targetTrType == "" || targetTrEndpoints == "" {
		return nil, fmt.Errorf("some nvme target info is missing, nqn: %s, type: %s, eindpoints: %s ", nqn, targetTrType, targetTrEndpoints)
	}

	endpoints := strings.Split(targetTrEndpoints, ",")
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints found in %s", volID)
	}

	return &nvmfDiskInfo{
		VolName:   volID,
		Endpoints: endpoints,
		Nqn:       nqn,
		Transport: targetTrType,
	}, nil
}

// getNVMfDiskMounter creates and configures a new disk mounter
func getNVMfDiskMounter(nvmfInfo *nvmfDiskInfo, targetPath string, cap *csi.VolumeCapability) *nvmfDiskMounter {
	return &nvmfDiskMounter{
		nvmfDiskInfo: nvmfInfo,
		isBlock:      cap.GetBlock() != nil,
		fsType:       cap.GetMount().GetFsType(),
		mountOptions: cap.GetMount().GetMountFlags(),
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()},
		exec:         exec.New(),
		targetPath:   targetPath,
		connector:    getNvmfConnector(nvmfInfo, targetPath),
	}
}

// getNVMfDiskUnMounter creates a new disk unmounter
func getNVMfDiskUnMounter() *nvmfDiskUnMounter {
	return &nvmfDiskUnMounter{
		mounter: mount.New(""),
		exec:    exec.New(),
	}
}

// AttachDisk connects to an NVMe-oF disk and returns the device path
func AttachDisk(volumeID string, connector *Connector) (string, error) {
	if connector == nil {
		return "", fmt.Errorf("connector is nil")
	}

	// connect nvmf target disk
	devicePath, err := connector.Connect()
	if err != nil {
		klog.Errorf("AttachDisk: VolumeID %s failed to connect, Error: %v", volumeID, err)
		return "", err
	}
	if devicePath == "" {
		klog.Errorf("AttachDisk: VolumeId %s return nil devicePath", volumeID)
		return "", fmt.Errorf("VolumeId %s return nil devicePath", volumeID)
	}

	klog.Infof("AttachDisk: Volume %s successfully connected, Device: %s", volumeID, devicePath)

	return devicePath, nil
}

// DetachDisk disconnects an NVMe-oF disk
func DetachDisk(targetNqn, targetPath string) error {
	connector := Connector{
		TargetNqn: targetNqn,
		HostNqn:   targetPath,
	}
	err := connector.Disconnect()
	if err != nil {
		klog.Errorf("DetachDisk: failed to disconnect, Error: %v", err)
		return err
	}

	return nil
}

// mountVolume handles both regular and block device mounts
func MountVolume(sourcePath string, nm *nvmfDiskMounter) error {
	klog.Infof("MountVolume: MntPath: %s, SrcPath: ", nm.targetPath, sourcePath)

	// Check if already mounted
	notMounted, err := nm.mounter.IsLikelyNotMountPoint(nm.targetPath)
	if err != nil && !os.IsNotExist(err) {
		klog.Errorf("MountVolume: failed to check mount point %s: %v", nm.targetPath, err)
		return fmt.Errorf("heuristic determination of mount point failed:%v", err)
	}
	if !notMounted {
		klog.Infof("mountVolume: %s is already mounted", nm.targetPath)
		return nil
	}

	if nm.isBlock {
		// Handle block device mount
		return mountBlockDevice(sourcePath, nm)
	} else {
		// Handle regular filesystem mount
		return mountFilesystem(sourcePath, nm)
	}
}

// UnmountVolume safely unmounts a volume
func UnmountVolume(targetPath string, unmounter *nvmfDiskUnMounter) error {
	// Check if already unmounted
	notMnt, err := unmounter.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			klog.Warningf("UnmountVolume: target path %s does not exist, skipping", targetPath)
			return nil
		}
		return fmt.Errorf("failed to check if %s is a mount point: %v", targetPath, err)
	}

	if notMnt {
		klog.V(4).Infof("UnmountVolume: %s is not a mount point, skipping", targetPath)
		return nil
	}

	// Unmount the volume
	klog.Infof("UnmountVolume: unmounting %s", targetPath)
	if err := unmounter.mounter.Unmount(targetPath); err != nil {
		klog.Errorf("UnmountVolume: failed to unmount %s: %v", targetPath, err)
		return fmt.Errorf("failed to unmount volume: %v", err)
	}

	return nil
}

// mountFilesystem handles mounting a formatted filesystem
func mountFilesystem(devicePath string, nm *nvmfDiskMounter) error {
	// Create mount point directory
	if err := os.MkdirAll(nm.targetPath, 0750); err != nil {
		klog.Errorf("mountFilesystem: failed to mkdir %s: %v", nm.targetPath, err)
		return err
	}

	// Mount the filesystem
	// Tips: use k8s mounter to mount fs and only support "ext4"
	var options []string
	options = append(options, nm.mountOptions...)
	klog.Infof("mountFilesystem: mounting %s at %s with fstype %s and options: %v", devicePath, nm.targetPath, nm.fsType, options)
	if err := nm.mounter.FormatAndMount(devicePath, nm.targetPath, nm.fsType, options); err != nil {
		klog.Errorf("mountFilesystem: failed to format and mount %s at %s: %v", devicePath, nm.targetPath, err)
		return fmt.Errorf("failed to format and mount device: %v", err)
	}

	return nil
}

// mountBlockDevice handles mounting a block device directly (without formatting)
func mountBlockDevice(devicePath string, nm *nvmfDiskMounter) error {
	// Create parent directory
	parentDir := filepath.Dir(nm.targetPath)
	if err := os.MkdirAll(parentDir, 0750); err != nil {
		klog.Errorf("mountBlockDevice: failed to create parent dir %s: %v", parentDir, err)
		return fmt.Errorf("failed to create parent directory: %v", err)
	}

	// Verify parent directory exists
	if exists, _ := mount.PathExists(parentDir); !exists {
		return fmt.Errorf("parent directory %s still does not exist after creation", parentDir)
	}

	// Create block device file
	pathFile, err := os.OpenFile(nm.targetPath, os.O_CREATE|os.O_RDWR, 0600) // #nosec G304
	if err != nil {
		klog.Errorf("mountBlockDevice: failed to create file %s: %v", nm.targetPath, err)
		return fmt.Errorf("failed to create block device file: %v", err)
	}
	if err = pathFile.Close(); err != nil {
		klog.Errorf("mountBlockDevice: failed to close file %s: %v", nm.targetPath, err)
		return fmt.Errorf("failed to close block device file: %v", err)
	}

	// Mount the block device with bind option
	options := append(nm.mountOptions, "bind")
	klog.Infof("mountBlockDevice: mounting %s at %s with options: %v", devicePath, nm.targetPath, options)
	if err := nm.mounter.Mount(devicePath, nm.targetPath, nm.fsType, options); err != nil {
		klog.Errorf("mountBlockDevice: failed to mount %s at %s: %v", devicePath, nm.targetPath, err)
		return fmt.Errorf("failed to mount block device: %v", err)
	}

	return nil
}
