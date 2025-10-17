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

type nvmfDiskInfo struct {
	VolName   string
	Nqn       string
	Addr      string
	Port      string
	DeviceID  string
	Transport string
	HostId    string
	HostNqn   string
}

func getNVMfDiskInfo(req *csi.NodePublishVolumeRequest) (*nvmfDiskInfo, error) {
	volName := req.GetVolumeId()

	volOpts := req.GetVolumeContext()
	targetTrAddr := volOpts["targetTrAddr"]
	targetTrPort := volOpts["targetTrPort"]
	targetTrType := volOpts["targetTrType"]
	devHostNqn := volOpts["hostNqn"]
	devHostId := volOpts["hostId"]
	deviceID := volOpts["deviceID"]
	if volOpts["deviceUUID"] != "" {
		if deviceID != "" {
			klog.Warningf("Warning: deviceUUID is overwriting already defined deviceID, volID: %s ", volName)
		}
		deviceID = strings.Join([]string{"uuid", volOpts["deviceUUID"]}, ".")
	}
	if volOpts["deviceEUI"] != "" {
		if deviceID != "" {
			klog.Warningf("Warning: deviceEUI is overwriting already defined deviceID, volID: %s ", volName)
		}
		deviceID = strings.Join([]string{"eui", volOpts["deviceEUI"]}, ".")
	}
	nqn := volOpts["nqn"]

	if targetTrAddr == "" || nqn == "" || targetTrPort == "" || targetTrType == "" || deviceID == "" {
		return nil, fmt.Errorf("some nvme target info is missing, volID: %s ", volName)
	}

	return &nvmfDiskInfo{
		VolName:   volName,
		Addr:      targetTrAddr,
		Port:      targetTrPort,
		Nqn:       nqn,
		DeviceID:  deviceID,
		Transport: targetTrType,
		HostNqn:   devHostNqn,
		HostId:    devHostId,
	}, nil
}

func AttachDisk(req *csi.NodePublishVolumeRequest, devicePath string) error {
	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()}

	targetPath := req.GetTargetPath()
	if req.GetVolumeCapability().GetBlock() != nil {
		dir := filepath.Dir(targetPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create dir of target block file: %s, err: %v", targetPath, err.Error())
			}
		}

		// bind mount
		file, err := os.Stat(targetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to stat target block file exist %s, err: %v", targetPath, err.Error())
			}

			newFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, 0750)
			if err != nil {
				return fmt.Errorf("failed to open target block file: %s, err: %v", targetPath, err.Error())
			}
			if err := newFile.Close(); err != nil {
				return fmt.Errorf("failed to close target block file: %s, err: %v", targetPath, err.Error())
			}
		} else {
			if file.Mode()&os.ModeDevice == os.ModeDevice {
				klog.Warning("AttachDisk Warning: Map skipped because bind mount already exist on the path: %v", targetPath)
				return nil
			}
		}
		if err := mounter.MountSensitive(devicePath, targetPath, "", []string{"bind"}, nil); err != nil {
			klog.Errorf("AttachDisk: failed to mount Device %s to %s, err: %v", devicePath, targetPath, err.Error())
			return fmt.Errorf("failed to mount Device %s to %s, err: %v", devicePath, targetPath, err.Error())
		}

		// symlink
		// if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		// 	return fmt.Errorf("failed to remove file %s: %v", targetPath, err)
		// }
		// if err := os.Symlink(devicePath, targetPath); err != nil {
		// 	klog.Errorf("AttachDisk: failed to link Device %s to %s, err: %v", devicePath, targetPath, err.Error())
		// 	return fmt.Errorf("failed to link Device %s to %s, err: %v", devicePath, targetPath, err.Error())
		// }

		// if _, err := os.Lstat(targetPath); err != nil {
		// 	klog.Errorf("Failed to verify symlink creation: %v", err)
		// 	return err
		// }
		// klog.Infof("Successfully created symlink from %s to %s", devicePath, targetPath)
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

	klog.Infof("AttachDisk: Successfully Attach Device %s to %s", devicePath, targetPath)
	return nil
}

func DetachDisk(targetPath string) (err error) {
	mounter := &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()}

	if notMnt, err := mount.IsNotMountPoint(mounter, targetPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("check target path error: %w", err)
		}
	} else if !notMnt {
		if err = mounter.Unmount(targetPath); err != nil {
			klog.Errorf("nvmf detach disk: failed to unmount: %s\nError: %v", targetPath, err)
			return err
		}

		// Delete the mount point
		if err = os.RemoveAll(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove target path: %w", err)
		}
	}

	return nil
}
