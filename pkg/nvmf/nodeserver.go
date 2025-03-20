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
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type NodeServer struct {
	Driver *driver
}

func NewNodeServer(d *driver) *NodeServer {
	return &NodeServer{
		Driver: d,
	}
}

func (n *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.Infof("Using Nvme NodeGetCapabilities")
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (n *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// 1. check parameters
	if req.GetVolumeCapability() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing Volume Capability in req.")
	}

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing VolumeID in req.")
	}

	if len(req.GetTargetPath()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing TargetPath in req.")
	}

	// 2. attachdisk
	nvmfInfo, err := getNVMfDiskInfo(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "NodePublishVolume: get NVMf disk info from req err: %v", err)
	}
	diskMounter := getNVMfDiskMounter(nvmfInfo, req)

	// attachDisk realize connect NVMf disk and mount to docker path
	_, err = AttachDisk(req, *diskMounter)
	if err != nil {
		klog.Errorf("NodePublishVolume: Attach volume %s with error: %s", req.VolumeId, err.Error())
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.Infof("NodeUnpublishVolume: Starting unpublish volume, %s, %v", req.VolumeId, req)

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume VolumeID must be provided")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Staging TargetPath must be provided")
	}
	targetPath := req.GetTargetPath()
	err := DetachDisk(req.VolumeId, getNVMfDiskUnMounter(req), targetPath)
	if err != nil {
		klog.Errorf("NodeUnpublishVolume: VolumeID: %s detachDisk err: %v", req.VolumeId, err)
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeStageVolume attaches the NVMe device to the node
func (n *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be required")
	}

	// Check for supported volume capabilities
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume capability is required")
	}

	// Check for staging target path
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging target path is required")
	}

	klog.V(4).Infof("NodeStageVolume called for volume %s", volumeID)

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume detaches the NVMe device from the node
func (n *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// Validate parameters
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Volume ID must be provided")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Staging target path must be provided")
	}

	klog.V(4).Infof("NodeUnstageVolume called for volume %s", req.VolumeId)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (n *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	deviceName, err := GetDeviceNameByVolumeID(req.VolumeId)
	if err != nil {
		klog.Errorf("NodeExpandVolume: Get Device by volumeID: %s error %v", req.VolumeId, err)
		return nil, status.Errorf(codes.Internal, "NodeExpandVolume: Get Device by volumeID: %s error %v", req.VolumeId, err)
	}

	scanPath := parseDeviceToControllerPath(deviceName)
	if utils.IsFileExisting(scanPath) {
		file, err := os.OpenFile(scanPath, os.O_RDWR|os.O_TRUNC, 0766)
		if err != nil {
			klog.Errorf("NodeExpandVolume: open scan path %s error: %v", scanPath, err)
			return nil, status.Errorf(codes.Internal, "NodeExpandVolume: open scan path %s error: %v", scanPath, err)
		}
		err = utils.WriteStringToFile(file, "1")
		if err != nil {
			klog.Errorf("NodeExpandVolume: Rescan error: %v", err)
			return nil, status.Errorf(codes.Internal, "NodeExpandVolume: Rescan error: %v", err)
		}
	} else {
		return nil, status.Errorf(codes.Internal, "NodeExpandVolume: rescan path %s not exist", scanPath)
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (n *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: n.Driver.nodeId,
	}, nil
}

func (n *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "NodeGetVolumeStats not implement")
}
