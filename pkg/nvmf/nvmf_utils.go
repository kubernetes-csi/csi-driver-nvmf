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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/utils"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"k8s.io/klog"
)

func waitForPathToExist(devicePath string, maxRetries, intervalSeconds int, deviceTransport string) (bool, error) {
	for i := 0; i < maxRetries; i++ {
		if deviceTransport == "tcp" {
			exist := utils.IsFileExisting(devicePath)
			if exist {
				return true, nil
			}
		} else {
			return false, fmt.Errorf("connect only support tcp")
		}

		if i == maxRetries-1 {
			break
		}
		time.Sleep(time.Second * time.Duration(intervalSeconds))
	}
	return false, fmt.Errorf("not found devicePath %s", devicePath)
}

func GetDeviceNameByVolumeID(volumeID string) (deviceName string, err error) {
	volumeLinkPath := strings.Join([]string{"/dev/disk/by-id/nvme-uuid", volumeID}, ".")
	stat, err := os.Lstat(volumeLinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("volumeID link path %q not found", volumeLinkPath)
		} else {
			return "", fmt.Errorf("error getting stat of %q: %v", volumeLinkPath, err)
		}
	}

	if stat.Mode()&os.ModeSymlink != os.ModeSymlink {
		klog.Errorf("volumeID link file %q found, but was not a symlink", volumeLinkPath)
		return "", fmt.Errorf("volumeID link file %q found, but was not a symlink", volumeLinkPath)
	}

	resolved, err := filepath.EvalSymlinks(volumeLinkPath)
	if err != nil {
		return "", fmt.Errorf("error reading target of symlink %q: %v", volumeLinkPath, err)
	}
	if !strings.HasPrefix(resolved, "/dev") {
		return "", fmt.Errorf("resolved symlink for %q was unexpected: %q", volumeLinkPath, resolved)
	}
	log.Infof("Device Link Info: %s link to %s", volumeLinkPath, resolved)
	tmp := strings.Split(resolved, "/")
	return tmp[len(tmp)-1], nil
}

func parseDeviceToControllerPath(deviceName string) string {
	nvmfControllerPrefix := "/sys/class/block"
	index := strings.LastIndex(deviceName, "n")
	parsed := deviceName[:index] + "c0" + deviceName[index:]
	scanPath := filepath.Join(nvmfControllerPrefix, parsed, "device/rescan_controller")
	return scanPath
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	klog.Infof("GRPC call: %s", info.FullMethod)
	klog.Infof("GRPC request: %s", protosanitizer.StripSecrets(req))
	resp, err := handler(ctx, req)
	if err != nil {
		klog.Errorf("GRPC error: %v", err)
	} else {
		klog.Infof("GRPC response: %s", protosanitizer.StripSecrets(resp))
	}
	return resp, err
}

func Rollback(err error, fc func()) {
	if err != nil {
		fc()
	}
}

func makeFile(pathname string) error {
	f, err := os.OpenFile(pathname, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	if err = f.Close(); err != nil {
		return err
	}
	return nil
}
