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
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// VolumeInfo wraps nvmfDiskInfo with allocation metadata
type VolumeInfo struct {
	*nvmfDiskInfo
	IsAllocated bool
}

// DeviceRegistry manages NVMe device discovery and allocation
type DeviceRegistry struct {
	Driver *driver

	// Protects device registry data
	mutex sync.RWMutex

	// All discovered volume info indexed by NQN
	devices map[string]*VolumeInfo

	// Set of available NQNs for quick lookup
	availableNQNs map[string]struct{}

	// Map from volume name to NQN for allocated devices
	volumeToNQN map[string]string

	// Tracks if initial sync from etcd has been performed
	initialSyncDone bool
}

// NewDeviceRegistry creates a new device registry
func NewDeviceRegistry(d *driver) *DeviceRegistry {
	return &DeviceRegistry{
		Driver:          d,
		devices:         make(map[string]*VolumeInfo),
		availableNQNs:   make(map[string]struct{}),
		volumeToNQN:     make(map[string]string),
		initialSyncDone: false,
	}
}

// EnsureInitialSync ensures the initial sync from Kubernetes API has been performed
func (r *DeviceRegistry) EnsureInitialSync(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.initialSyncDone {
		return nil
	}

	klog.V(4).Info("Performing initial sync from existing PersistentVolumes")

	if err := r.SyncFromPV(ctx); err != nil {
		return fmt.Errorf("failed to sync from Kubernetes API: %v", err)
	}

	r.initialSyncDone = true

	klog.V(4).Infof("Successfully synced %d volumes from Kubernetes API", len(r.devices))
	return nil
}

// SyncFromPV synchronizes volume allocation data from Kubernetes API server to the local device registry.
// It retrieves all PersistentVolumes provisioned by this driver through the Kubernetes API and updates
// the registry accordingly, ensuring the controller's state reflects the actual allocations in the cluster.
func (r *DeviceRegistry) SyncFromPV(ctx context.Context) error {
	list, err := r.Driver.kubeClient.
		CoreV1().
		PersistentVolumes().
		List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list PersistentVolumes: %v", err)
		return err
	}

	for _, pv := range list.Items {
		// Check if the PV was provisioned by this driver using annotations
		provisionedBy, exists := pv.Annotations["pv.kubernetes.io/provisioned-by"]
		if !exists || provisionedBy != r.Driver.name {
			continue
		}

		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == r.Driver.name {
			nqn := pv.Spec.CSI.VolumeHandle
			if nqn, exists := r.volumeToNQN[pv.Name]; exists {
				klog.Errorf("Volume %s is already existing in the registry with NQN %s", pv.Name, nqn)
				continue
			}

			// Update the volume info with the allocated device
			r.devices[nqn] = &VolumeInfo{
				nvmfDiskInfo: &nvmfDiskInfo{
					VolName:   pv.Name,
					Nqn:       nqn,
					Transport: pv.Spec.CSI.VolumeAttributes[paramType],
					Endpoints: strings.Split(pv.Spec.CSI.VolumeAttributes["targetTrEndpoint"], ","),
				},
				IsAllocated: true,
			}

			klog.V(4).Infof("Recovered device mapping: [PV] %s → [Device NQN] %s", pv.Name, nqn)
			r.volumeToNQN[pv.Name] = nqn

		}
	}

	return nil
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
		klog.V(4).Infof("- NQN: %s, isAllocated: %t, Endpoints: %v", device.Nqn, device.IsAllocated, device.Endpoints)
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

	klog.V(4).Infof("[%d/%d] Allocated volume %s (NQN %s)", len(r.devices)-len(r.availableNQNs), len(r.devices), volumeName, nqn)

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

	klog.V(4).Infof("[%d/%d] Released volume %s", len(r.devices)-len(r.availableNQNs), len(r.devices), nqn)
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

	// Discover devices on each port
	ips := strings.Split(targetAddr, ",")
	ports := strings.Split(targetPort, ",")
	// collect devices by NQN with endpoints as a list
	deviceMap := make(map[string]*nvmfDiskInfo)
	for _, ip := range ips {
		ip = strings.TrimSpace(ip) // Trim spaces in case there are spaces after commas
		for _, port := range ports {
			port = strings.TrimSpace(port) // Trim spaces in case there are spaces after commas
			if port == "" {
				continue
			}

			klog.V(4).Infof("Running discovery on %s://%s:%s", targetType, ip, port)
			cmd := exec.Command("nvme", "discover", "-a", ip, "-s", port, "-t", targetType, "-o", "json")
			var out bytes.Buffer
			cmd.Stdout = &out

			if err := cmd.Run(); err != nil {
				klog.Warningf("nvme discover command failed for port %s: %v", port, err)
				continue // Continue with next port instead of failing completely
			}

			// Parse JSON output and organize by NQN
			devices := parseNvmeDiscoveryOutput(out.String(), targetType)
			for _, device := range devices {
				if existingDevice, exists := deviceMap[device.Nqn]; exists {
					// NQN already exists, just add the new endpoint if it's not already in the list
					endpoint := device.Endpoints[0]
					endpointExists := false
					for _, existingEndpoint := range existingDevice.Endpoints {
						if existingEndpoint == endpoint {
							endpointExists = true
							break
						}
					}
					if !endpointExists {
						existingDevice.Endpoints = append(existingDevice.Endpoints, endpoint)
					}
				} else {
					// New NQN, add the device to the map
					deviceMap[device.Nqn] = device
				}
			}
		}
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

		// Set endpoint address if both Addr and Port are provided
		// These are required for the noder server to connect to the target with multipath
		if record.Addr != "" && record.Port != "" {
			record.Endpoints = []string{record.Addr + ":" + record.Port}
		} else {
			klog.Warningf("Skipping record with invalid Addr or Port: Addr=%s, Port=%s", record.Addr, record.Port)
		}

		// Append to targets list
		recordCopy := record // Create a copy because 'record' is reused in each loop iteration
		targets = append(targets, &recordCopy)
	}
	return targets
}
