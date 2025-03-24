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
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

func waitForPathToExist(devicePath string, maxRetries, intervalSeconds int, deviceTransport string) (bool, error) {
	for i := 0; i < maxRetries; i++ {
		exist := utils.IsFileExisting(devicePath)
		if exist {
			return true, nil
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
	klog.Infof("Device Link Info: %s link to %s", volumeLinkPath, resolved)
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

// findPathWithRetry waits until the NVMe device with the specified NQN is fully connected
// and returns its device path. It retries up to maxRetries times with intervalSeconds between attempts.
func findPathWithRetry(targetNqn string, maxRetries, intervalSeconds int32) (string, error) {
	for i := int32(0); i < maxRetries; i++ {
		time.Sleep(time.Second * time.Duration(intervalSeconds))

		// Step 1: Find the device name
		deviceName := getDeviceNameBySubNqn(targetNqn)
		if deviceName == "" {
			if i == maxRetries-1 {
				klog.Infof("Failed to find device name for target NQN %s after %d attempts", targetNqn, maxRetries)
				break
			}
			continue
		}

		// Step 2: Find the UUID
		uuid := getDeviceUUID(deviceName)
		if uuid == "" {
			if i == maxRetries-1 {
				klog.Infof("Failed to find UUID for device %s after %d attempts", deviceName, maxRetries)
				break
			}
			continue
		}

		// Step 3: Check if device path exists
		devicePath := strings.Join([]string{"/dev/disk/by-id/nvme-uuid", uuid}, ".")
		if exists := utils.IsFileExisting(devicePath); !exists {
			if i == maxRetries-1 {
				klog.Infof("Device path %s does not exist after %d attempts", devicePath, maxRetries)
				break
			}
			continue
		}

		// All steps successful
		klog.Infof("Found device path %s for target NQN %s", devicePath, targetNqn)
		return devicePath, nil
	}

	return "", fmt.Errorf("device for target NQN %s not ready after %d attempts",
		targetNqn, maxRetries)
}

// getDeviceNameBySubNqn finds a device's name based on its subsystem NQN
func getDeviceNameBySubNqn(targetNqn string) string {
	devices, err := os.ReadDir(SYS_NVMF)
	if err != nil {
		klog.Errorf("Failed to read NVMe devices directory: %v", err)
		return ""
	}

	for _, device := range devices {
		subsysNqnPath := fmt.Sprintf("%s/%s/subsysnqn", SYS_NVMF, device.Name())

		file, err := os.Open(subsysNqnPath)
		if err != nil {
			continue
		}
		defer file.Close()

		lines, err := utils.ReadLinesFromFile(file)
		if err != nil || len(lines) == 0 {
			continue
		}

		if lines[0] == targetNqn {
			return device.Name()
		}
	}

	return ""
}

// getDeviceUUID returns the UUID for the given device name
func getDeviceUUID(deviceName string) string {
	// Try uuid first, then nguid
	identifierTypes := []string{"uuid", "nguid"}

	for _, idType := range identifierTypes {
		identifier, err := getDeviceIdentifierFromSysfs(deviceName, idType)
		if err == nil {
			return identifier
		}
	}

	return ""
}

// getDeviceIdentifierFromSysfs extracts device identifiers from sysfs
func getDeviceIdentifierFromSysfs(deviceName string, identifierType string) (string, error) {
	// Find namespaces - supports both standard (nvme0n1) and controller-based (nvme2c2n1) namespaces
	namespacePattern := filepath.Join(SYS_NVMF, deviceName, "nvme*n*")
	namespaces, err := filepath.Glob(namespacePattern)
	if err != nil || len(namespaces) == 0 {
		return "", fmt.Errorf("no namespace found for device %s: %v", deviceName, err)
	}

	nsDir := filepath.Base(namespaces[0])
	identifierPath := filepath.Join(SYS_NVMF, deviceName, nsDir, identifierType)

	if _, err := os.Stat(identifierPath); os.IsNotExist(err) {
		return "", fmt.Errorf("%s file does not exist for device %s", identifierType, deviceName)
	}

	data, err := os.ReadFile(identifierPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s from sysfs: %v", identifierType, err)
	}

	identifier := strings.TrimSpace(string(data))
	if identifier == "" {
		return "", fmt.Errorf("empty %s for device %s", identifierType, deviceName)
	}

	// Convert NGUID to UUID if applicable
	if identifierType == "nguid" {
		identifier := strings.ReplaceAll(identifier, " ", "")
		if len(identifier) != 32 {
			return "", fmt.Errorf("invalid NGUID length: got %d, expected 32", len(identifier))
		}

		return fmt.Sprintf("%s-%s-%s-%s-%s",
			identifier[0:8],
			identifier[8:12],
			identifier[12:16],
			identifier[16:20],
			identifier[20:]), nil
	}

	return identifier, nil
}
