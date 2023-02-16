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
	"path"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
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
		},
	}, nil
}

func (n *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// Pre-check
	if req.GetVolumeCapability() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing Volume Capability in req.")
	}

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing VolumeID in req.")
	}

	if len(req.GetTargetPath()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "NodePublishVolume missing TargetPath in req.")
	}

	klog.Infof("VolumeID %s publish to targetPath %s.", req.GetVolumeId(), req.GetTargetPath())
	// Connect remote disk
	nvmfInfo, err := getNVMfDiskInfo(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "NodePublishVolume: get NVMf disk info from req err: %v", err)
	}

	connector := getNvmfConnector(nvmfInfo)
	devicePath, err := connector.Connect()

	connectorFilePath := path.Join(DefaultVolumeMapPath, req.GetVolumeId()+".json")

	if err != nil {
		klog.Errorf("VolumeID %s failed to connect, Error: %v", req.VolumeId, err)
		return nil, status.Errorf(codes.Internal, "VolumeID %s failed to connect, Error: %v", req.VolumeId, err)
	}
	if devicePath == "" {
		klog.Errorf("VolumeID %s connected, but return nil devicePath", req.VolumeId)
		return nil, status.Errorf(codes.Internal, "VolumeID %s connected, but return nil devicePath", req.VolumeId)
	}
	klog.Infof("Volume %s successful connected, Device：%s", req.VolumeId, devicePath)
	defer Rollback(err, func() {
		connector.Disconnect()
	})

	err = persistConnectorFile(connector, connectorFilePath)
	if err != nil {
		klog.Errorf("failed to persist connection info: %v", err)
		return nil, status.Errorf(codes.Internal, "VolumeID %s persist connection info error: %v", req.VolumeId, err)
	}

	// Attach disk to container path
	if req.GetVolumeCapability().GetBlock() != nil && req.GetVolumeCapability().GetMount() != nil {
		return nil, status.Errorf(codes.InvalidArgument, "cannot have both block and mount access type")
	}

	defer Rollback(err, func() {
		removeConnectorFile(connectorFilePath)
	})

	err = AttachDisk(req, devicePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "VolumeID %s attach error: %v", req.VolumeId, err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.Infof("NodeUnpublishVolume: Starting unpublish volume, %s, %v", req.VolumeId, req)

	// Pre-check
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume VolumeID must be provided")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Staging TargetPath must be provided")
	}

	// Detach disk
	targetPath := req.GetTargetPath()
	err := DetachDisk(req.VolumeId, targetPath)
	if err != nil {
		klog.Errorf("VolumeID: %s detachDisk err: %v", req.VolumeId, err)
		return nil, err
	}

	// Disconnect remote disk
	connectorFilePath := path.Join(DefaultVolumeMapPath, req.GetVolumeId()+".json")
	connector, err := GetConnectorFromFile(connectorFilePath)
	if err != nil {
		klog.Errorf("failed to get connector from path %s Error: %v", targetPath, err)
		return nil, status.Errorf(codes.Internal, "failed to get connector from path %s Error: %v", targetPath, err)
	}
	err = connector.Disconnect()
	if err != nil {
		klog.Errorf("VolumeID: %s failed to disconnect, Error: %v", targetPath, err)
		return nil, status.Errorf(codes.Internal, "failed to get connector from path %s Error: %v", targetPath, err)
	}
	removeConnectorFile(connectorFilePath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (n *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (n *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
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
