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
	"k8s.io/utils/mount"
)

func AttachDisk(req *csi.NodePublishVolumeRequest, nm nvmfDiskMounter) (string, error) {
	devicePath, err := Connect(nm.connector)
	if err != nil {
		klog.Errorf("AttachDisk: VolumeID %s failed to connect, Error: %v", req.VolumeId, err)
		return "", err
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

	if err := os.MkdirAll(mntPath, 0750); err != nil {
		klog.Errorf("AttachDisk: failed to mkdir %s, error", mntPath)
		return "", err
	}

	err = persistConnector(nm.connector, mntPath+".json")
	if err != nil {
		klog.Errorf("AttachDisk: failed to persist connection info: %v", err)
		klog.Errorf("AttachDisk: disconnecting volume and failing the publish request because persistence file are required for unpublish volume")
		return "", fmt.Errorf("unable to create persistence file for connection")
	}

	// Tips: use k8s mounter to mount fs and only support "ext4"
	var options []string
	if nm.readOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}
	options = append(options, nm.mountOptions...)
	err = nm.mounter.FormatAndMount(devicePath, mntPath, nm.fsType, options)
	if err != nil {
		klog.Errorf("AttachDisk: failed to mount Device %s to %s with options: %v", devicePath, mntPath, options)
		Disconnect(nm.connector)
		removeConnector(mntPath)
		return "", fmt.Errorf("failed to mount Device %s to %s with options: %v", devicePath, mntPath, options)
	}

	klog.Infof("AttachDisk: Successfully Mount Device %s to %s with options: %v", devicePath, mntPath, options)
	return devicePath, nil
}

func DetachDisk(volumeID string, num *nvmfDiskUnMounter, targetPath string) error {
	_, cnt, err := mount.GetDeviceNameFromMount(num.mounter, targetPath)
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
	if err = num.mounter.Unmount(targetPath); err != nil {
		klog.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}
	cnt--
	if cnt != 0 {
		return nil
	}

	connector, err := GetConnectorFromFile(targetPath + ".json")
	if err != nil {
		klog.Errorf("DetachDisk: failed to get connector from path %s Error: %v", targetPath, err)
		return err
	}
	err = Disconnect(connector)
	if err != nil {
		klog.Errorf("DetachDisk: VolumeID: %s failed to disconnect, Error: %v", volumeID, err)
		return err
	}
	removeConnector(targetPath)
	return nil
}
