/*
Copyright 2025 The Kubernetes Authors.

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
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// VolumeInfo wraps nvmfDiskInfo with allocation metadata
type VolumeInfo struct {
	*nvmfDiskInfo
	IsAllocated bool
}

// DeviceRegistry manages NVMe device discovery and allocation
type DeviceRegistry struct {
	// Protects device registry data
	mutex sync.RWMutex

	// All discovered volume info indexed by NQN
	devices map[string]*VolumeInfo

	// Set of available NQNs for quick lookup
	availableNQNs map[string]struct{}

	// Map from volume name to NQN for allocated devices
	volumeToNQN map[string]string
}

// NewDeviceRegistry creates a new device registry
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices:         make(map[string]*VolumeInfo),
		availableNQNs:   make(map[string]struct{}),
		volumeToNQN:     make(map[string]string),
	}
}

// DiscoverDevices performs NVMe device discovery
func (r *DeviceRegistry) DiscoverDevices(params map[string]string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	klog.V(4).Info("Performing NVMe device discovery")
	discoveredDevices, err := discoverNVMeDevices(params)
	if err != nil {
		return fmt.Errorf("device discovery failed: %v", err)
	}

	if len(discoveredDevices) == len(r.devices) {
		klog.V(4).Info("No new devices discovered, skipping update")
		return nil
	}

	for nqn, diskInfo := range discoveredDevices {
		if _, exists := r.devices[nqn]; !exists {
			r.devices[nqn] = &VolumeInfo{
				nvmfDiskInfo: diskInfo,
				IsAllocated:  false,
			}

			r.availableNQNs[nqn] = struct{}{}
		}
	}
	klog.V(4).Infof("Discovered %d NVMe targets", len(r.devices))

	for _, device := range r.devices {
		klog.V(4).Infof("- NQN: %s, Endpoints: %v:%v", device.Nqn, device.Addr, device.Port)
	}

	return nil
}

// AllocateDevice selects and allocates a device for a volume
func (r *DeviceRegistry) AllocateDevice(volumeName string) (*VolumeInfo, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check this volume is already allocated
	if nqn, exists := r.volumeToNQN[volumeName]; exists {
		return nil, fmt.Errorf("already allocated. PV: %s, device: %s", volumeName, nqn)
	}

	// Check if any devices are available
	if len(r.availableNQNs) == 0 {
		return nil, fmt.Errorf("no available devices found")
	}

	var nqn string
	for n := range r.availableNQNs {
		if r.devices[n].IsAllocated {
			klog.Errorf("Device %s is marked as available but is already allocated. Device details: %+v", n, r.devices[n])
			continue
		}

		nqn = n
		break
	}

	if nqn == "" {
		return nil, fmt.Errorf("no available devices found")
	}

	// Update tracking maps
	delete(r.availableNQNs, nqn)
	r.volumeToNQN[volumeName] = nqn
	device := r.devices[nqn]
	device.VolName = volumeName
	device.IsAllocated = true

	klog.V(4).Infof("[%d/%d] Allocated volume %s (NQN %s)", len(r.devices) - len(r.availableNQNs), len(r.devices), volumeName, nqn)

	return device, nil
}

// ReleaseDevice releases a device allocation
func (r *DeviceRegistry) ReleaseDevice(nqn string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()


	device, exists := r.devices[nqn]
	if !exists {
		// CSI spec requires idempotency: return success even if volume doesn't exist
		// This allows safe retries and prevents errors when volume was already deleted
		klog.Infof("Volume %s not found", nqn)
		return
	}

	// Update tracking maps
	device.IsAllocated = false
	delete(r.volumeToNQN, device.VolName)
	r.availableNQNs[nqn] = struct{}{}
	device.VolName = ""

	klog.V(4).Infof("[%d/%d] Released volume %s", len(r.devices) - len(r.availableNQNs), len(r.devices), nqn)
}

// discoverNVMeDevices runs NVMe discovery and returns available targets
func discoverNVMeDevices(params map[string]string) (map[string]*nvmfDiskInfo, error) {
	if params == nil {
		return nil, fmt.Errorf("discovery parameters are nil")
	}

	targetAddr := params[paramAddr]
	targetPort := params[paramPort]
	targetType := params[paramType]

	if targetAddr == "" || targetPort == "" || targetType == "" {
		return nil, fmt.Errorf("missing required discovery parameters")
	}

	if strings.ToLower(targetType) != "tcp" && strings.ToLower(targetType) != "rdma" {
		return nil, fmt.Errorf("transport type must be tcp or rdma, got: %s", targetType)
	}

	klog.V(4).Infof("Discovering NVMe targets at %s:%s using %s", targetAddr, targetPort, targetType)

	deviceMap := make(map[string]*nvmfDiskInfo)
	cmd := exec.Command("nvme", "discover", "-a", targetAddr, "-s", targetPort, "-t", targetType, "-o", "json")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nvme discover command failed: %v", err)
	}

	// Parse JSON output and organize by NQN
	devices := parseNvmeDiscoveryOutput(out.String(), targetType)
	for _, device := range devices {
		deviceMap[device.Nqn] = device
	}
	
	return deviceMap, nil
}

// parseNvmeDiscoveryOutput parses the JSON output of nvme discover command
func parseNvmeDiscoveryOutput(output string, targetType string) []*nvmfDiskInfo {
	targets := make([]*nvmfDiskInfo, 0)
	discoveryNQN := "discovery"

	// Define structure for JSON parsing
	type discoveryResponse struct {
		Records []nvmfDiskInfo `json:"records"`
	}

	var response discoveryResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		klog.Errorf("Failed to parse NVMe discovery JSON output: %v", err)
		return targets
	}

	for _, record := range response.Records {
		// Skip discovery NQN and non-matching transport type
		if strings.Contains(strings.ToLower(record.Nqn), discoveryNQN) ||
			record.Transport != targetType {
			continue
		}

		// Append to targets list
		recordCopy := record // Create a copy because 'record' is reused in each loop iteration
		targets = append(targets, &recordCopy)
	}

	return targets
}
