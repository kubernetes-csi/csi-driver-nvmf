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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	UseActualDeviceCapacity int64 = 0 // Use the actual device capacity
)

type ControllerServer struct {
	Driver         *driver
	deviceRegistry *DeviceRegistry
}

// create controller server
func NewControllerServer(d *driver) *ControllerServer {
	return &ControllerServer{
		Driver:         d,
		deviceRegistry: NewDeviceRegistry(),
	}
}

// CreateVolume provisions a new volume
func (c *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volumeName := req.GetName()
	if !isValidVolumeName(volumeName) {
		return nil, status.Error(codes.InvalidArgument, "volume Name must be provided")
	}

	cap := req.GetVolumeCapabilities()
	if !isValidVolumeCapabilities(cap) {
		return nil, status.Error(codes.InvalidArgument, "volume Capabilities are invalid")
	}

	klog.V(4).Infof("CreateVolume called with name: %s", volumeName)

	// Extract volume parameters
	parameters := req.GetParameters()
	if parameters == nil {
		parameters = make(map[string]string)
	}

	// Discover NVMe devices if needed
	if err := c.deviceRegistry.DiscoverDevices(parameters); err != nil {
		klog.Errorf("Failed to discover NVMe devices: %v", err)
		return nil, status.Errorf(codes.Internal, "device discovery failed: %v", err)
	}

	// Acquire volume lock to prevent concurrent operations
	if acquired := c.Driver.volumeLocks.TryAcquire(volumeName); !acquired {
		return nil, status.Errorf(codes.Aborted, "concurrent operation in progress for volume: %s", volumeName)
	}
	defer c.Driver.volumeLocks.Release(volumeName)

	// Allocate a device
	allocatedDevice, err := c.deviceRegistry.AllocateDevice(volumeName)
	if err != nil {
		klog.Errorf("Failed to allocate device for volume %s: %v", volumeName, err)
		return nil, status.Errorf(codes.ResourceExhausted, "no suitable device available: %v", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      allocatedDevice.Nqn,
			CapacityBytes: UseActualDeviceCapacity, // PV will use the actual capacity
			VolumeContext: parameters,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

// DeleteVolume deletes a volume
func (c *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if !isValidVolumeID(volumeID) {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}

	klog.V(4).Infof("DeleteVolume called for volume ID %s", volumeID)

	// Acquire lock to prevent concurrent operations on this volume
	if acquired := c.Driver.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, "concurrent operation in progress for volume: %s", volumeID)
	}
	defer c.Driver.volumeLocks.Release(volumeID)

	// Find the volume by NQN
	// Note: volumeID is expected to be in NQN (NVMe Qualified Name) format.
	// This assumption is valid because in CreateVolume, we assigned the device's NQN
	// as the volumeID when returning the CreateVolumeResponse.
	nqn := volumeID
	c.deviceRegistry.ReleaseDevice(nqn)

	return &csi.DeleteVolumeResponse{}, nil
}

func (c *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ControllerExpandVolume should implement by yourself")
}

func (c *ControllerServer) ControllerGetVolume(ctx context.Context, request *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ControllerGetVolume not implement")
}

func (c *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ControllerPublishVolume not implement")
}

func (c *ControllerServer) ControllerUnpublishVolume(ctx context.Context, request *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ControllerUnpublishVolume not implement")
}

func (c *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, request *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ValidateVolumeCapabilities not implement")
}

func (c *ControllerServer) ListVolumes(ctx context.Context, request *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ListVolumes not implement")
}

func (c *ControllerServer) GetCapacity(ctx context.Context, request *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "GetCapacity not implement")
}

func (c *ControllerServer) ControllerGetCapabilities(ctx context.Context, request *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.Infof("Using default ControllerGetCapabilities")

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: c.Driver.cscap,
	}, nil
}

func (c *ControllerServer) CreateSnapshot(ctx context.Context, request *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "CreateSnapshot not implement")
}

func (c *ControllerServer) DeleteSnapshot(ctx context.Context, request *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "DeleteSnapshot not implement")
}

func (c *ControllerServer) ListSnapshots(ctx context.Context, request *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "ListSnapshots not implement")
}

func isValidVolumeName(volumeName string) bool {
	if volumeName == "" {
		klog.Error("Volume Name cannot be empty")
		return false
	}

	return true
}

func isValidVolumeID(volumeID string) bool {
	if volumeID == "" {
		klog.Error("Volume ID cannot be empty")
		return false
	}

	return true
}

func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) bool {
	if len(volCaps) == 0 {
		klog.Error("Volume Capabilities not provided")
		return false
	}

	for _, cap := range volCaps {
		if cap.GetBlock() != nil && cap.GetMount() != nil {
			klog.Error("Cannot specify both block and mount access types")
			return false
		}
		if cap.GetBlock() == nil && cap.GetMount() == nil {
			klog.Error("Must specify either block or mount access type")
			return false
		}
	}

	return true
}
