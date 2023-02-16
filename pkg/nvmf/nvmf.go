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
	"k8s.io/klog"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

type nvmfDiskInfo struct {
	VolName    string
	Nqn        string
	Addr       string
	Port       string
	DeviceUUID string
	Transport  string
}

func getNVMfDiskInfo(req *csi.NodePublishVolumeRequest) (*nvmfDiskInfo, error) {
	volName := req.GetVolumeId()

	volOpts := req.GetVolumeContext()
	targetTrAddr := volOpts["targetTrAddr"]
	targetTrPort := volOpts["targetTrPort"]
	targetTrType := volOpts["targetTrType"]
	deviceUUID := volOpts["deviceUUID"]
	nqn := volOpts["nqn"]

	if targetTrAddr == "" || nqn == "" || targetTrPort == "" || targetTrType == "" || deviceUUID == "" {
		return nil, fmt.Errorf("Some Nvme target info is missing, volID: %s ", volName)
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

func AttachDisk(req *csi.NodePublishVolumeRequest, devicePath string) error {
	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()}

	targetPath := req.GetTargetPath()

	if req.GetVolumeCapability().GetBlock() != nil {
		_, err := os.Lstat(targetPath)
		if os.IsNotExist(err) {
			if err = makeFile(targetPath); err != nil {
				return fmt.Errorf("failed to create target path, err: %s", err.Error())
			}
		}
		if err != nil {
			return fmt.Errorf("failed to check if the target block file exist, err: %s", err.Error())
		}

		notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("error checking path %s for mount: %w", targetPath, err)
			}
			notMounted = true
		}

		if !notMounted {
			klog.Infof("VolumeID: %s, Path: %s is already mounted, device: %s", req.VolumeId, targetPath, devicePath)
			return nil
		}

		options := []string{""}
		if err = mounter.Mount(devicePath, targetPath, "", options); err != nil {
			klog.Errorf("AttachDisk: failed to mount Device %s to %s with options: %v", devicePath, targetPath, options)
			return fmt.Errorf("failed to mount Device %s to %s with options: %v", devicePath, targetPath, options)
		}

	} else if req.GetVolumeCapability().GetMount() != nil {
		notMounted, err := mounter.IsLikelyNotMountPoint(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				klog.Errorf("AttachDisk: VolumeID: %s, Path %s is not exist, so create one.", req.GetVolumeId(), req.GetTargetPath())
				if err = os.MkdirAll(targetPath, 0750); err != nil {
					return fmt.Errorf("create target path: %v", err)
				}
				notMounted = true
			} else {
				return fmt.Errorf("check target path %v", err)
			}
		}

		if !notMounted {
			klog.Infof("AttachDisk: VolumeID: %s, Path: %s is already mounted.", req.GetVolumeId(), req.GetTargetPath())
			return nil
		}

		fsType := req.GetVolumeCapability().GetMount().GetFsType()
		readonly := req.GetReadonly()
		mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
		options := []string{""}
		if readonly {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		options = append(options, mountOptions...)

		if err = mounter.FormatAndMount(devicePath, targetPath, fsType, options); err != nil {
			klog.Errorf("AttachDisk: failed to mount Device %s to %s with options: %v", devicePath, targetPath, options)
			return fmt.Errorf("failed to mount Device %s to %s with options: %v", devicePath, targetPath, options)
		}
	}
	return nil
}

func DetachDisk(volumeID string, targetPath string) error {
	mounter := mount.New("")

	_, cnt, err := mount.GetDeviceNameFromMount(mounter, targetPath)
	if err != nil {
		klog.Errorf("nvmf detach disk: failed to get device from mnt: %s\nError: %v", targetPath, err)
		return err
	}
	if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExists {
		klog.Warningf("Warning: Unmount skipped because path does not exist: %v", targetPath)
		return nil
	}
	if err = mounter.Unmount(targetPath); err != nil {
		klog.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}
	cnt--
	if cnt != 0 {
		return nil
	}

	return nil
}
