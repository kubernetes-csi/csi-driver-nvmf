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
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

// the map of VolumeName to PvName for idempotency.
var createdVolumeMap = map[string]*csi.Volume{}

// the map of DeleteVolumeReq for idempotency
var deletingVolumeReqMap = map[string]*csi.DeleteVolumeRequest{}

// the map of CreateVolumeReq for idempotency
var creatingVolumeReqMap = map[string]*csi.CreateVolumeRequest{}

type ControllerServer struct {
	Driver *driver
}

// create controller server
func NewControllerServer(d *driver) *ControllerServer {
	return &ControllerServer{
		Driver: d,
	}
}

func (c *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	//1. check parameters
	if err := c.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: NVMf Driver not support create volume.")
	}

	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: volume's name cannot be empty.")
	}

	if req.GetVolumeCapabilities() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: volume %s's capabilities cannot be empty.", req.GetName())
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: volume %s's capacityRange cannot be empty.", req.GetName())
	}

	volSizeBytes := int64(req.GetCapacityRange().RequiredBytes)

	createVolArgs := req.GetParameters()
	createVolReq, err := ParseCreateVolumeParameters(createVolArgs)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: volume %s's parameters parsed error: %s", req.GetName(), err)
	}

	// 2. if created, return created volume info
	oldVol, ok := createdVolumeMap[req.GetName()]
	if ok {
		// todo: check more vol context like permission?
		// oldVolContext := oldVol.GetVolumeContext()
		klog.Warningf("CreateVolume: volume %s has already created and volumeId: %s", req.GetName(), oldVol.VolumeId)
		if oldVol.CapacityBytes != volSizeBytes {
			return nil, status.Errorf(codes.InvalidArgument, "CreateVolume: the exist vol-%s's size %d is different from the requested volume %s's size %d.", oldVol.VolumeId, oldVol.CapacityBytes, req.GetName(), volSizeBytes)
		}
		return &csi.CreateVolumeResponse{
			Volume: oldVol,
		}, nil
	}

	// 3. if creating, return error
	_, ok = creatingVolumeReqMap[req.GetName()]
	if ok {
		klog.Warningf("CreateVolume: volume %s has been creating in other req", req.GetName())
		//maybe return err?
		return &csi.CreateVolumeResponse{}, nil
	}

	creatingVolumeReqMap[req.GetName()] = req

	// 4. if not created, request backend controller to create a new volume
	createVolReq.Name = req.GetName()
	createVolReq.SizeByte = req.GetCapacityRange().RequiredBytes

	volReqBody, err := json.Marshal(createVolReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: json marshal error: %s", err)
	}

	response := c.Driver.client.Post().Action("/volume/create").Body(volReqBody).Do()
	klog.Infof("CreateVolume:create volume backend response's statuscode: %d, res: %s", response.StatusCode(), string(response.Body()))

	if err := response.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: backend response error: %s", err)
	}

	if response.StatusCode() != http.StatusOK {
		return nil, status.Errorf(codes.Internal, "CreateVolume: backend has reponse but failed for volume %s", req.GetName())
	}

	var createVolumeResponse client.CreateVolumeResponse
	err = response.Parse(&createVolumeResponse)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVolume: response parse error: %s", err)
	}

	klog.Infof("CreateVolume: createVolume success for volume %s, volume ID: %s", req.GetName(), createVolumeResponse.ID)
	volContext, _ := GetVolumeContext(&createVolumeResponse)

	tmpVolume := &csi.Volume{
		VolumeId:      createVolumeResponse.ID,
		CapacityBytes: int64(createVolumeResponse.CapacityBytes),
		VolumeContext: volContext,
	}

	createdVolumeMap[createVolumeResponse.ID] = tmpVolume
	err = PersistVolumeInfo(tmpVolume, filepath.Join(c.Driver.volumeMapDir, tmpVolume.GetVolumeId()))
	if err != nil {
		klog.Warningf("CreateVolume: create volume %s success, but persistent error: %s", req.GetName(), err)
	}
	delete(creatingVolumeReqMap, req.GetName())

	return &csi.CreateVolumeResponse{Volume: tmpVolume}, nil
}

func (c *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// 1. check parameters
	if err := c.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "DeleteVolume: NVMf Driver not support delete volume.")
	}

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "DeleteVolume: delete req's volume ID can't be empty.")
	}

	if req.GetSecrets() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "DeleteVolume: delete req's secrets can't be empty")
	}

	var volume *csi.Volume
	var err error

	volumeMapFilePath := filepath.Join(c.Driver.volumeMapDir, req.GetVolumeId())

	// 2. if deleting, return error
	_, ok := deletingVolumeReqMap[req.GetVolumeId()]
	if ok {
		klog.Warningf("DeleteVolume: vol-%s has been deleting in other req", req.GetVolumeId())
		// maybe return error?
		return &csi.DeleteVolumeResponse{}, nil
	}
	deletingVolumeReqMap[req.GetVolumeId()] = req

	volume, ok = createdVolumeMap[req.GetVolumeId()]
	if !ok {
		klog.Errorf("DeleteVolume: can't find the vol-%s in driver cache", req.GetVolumeId())
		volume, err = GetVolumeInfoFromFile(volumeMapFilePath)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "DeleteVolume: can't get vol-%s info from cache or file, not exist.", req.GetVolumeId())
		}
		klog.Warningf("DeleteVolume: get vol-%s from file.", req.GetVolumeId())
	}

	//todo: P0-delete should add some permission check.
	deleteVolReq := &DeleteVolumeRequest{
		VolumeId: volume.VolumeId,
	}

	deleteVolBody, err := json.Marshal(deleteVolReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "DeletedVolume: json marshal error: %s", err)
	}

	response := c.Driver.client.Post().Action("/volume/delete").Body(deleteVolBody).Do()
	klog.Infof("DeleteVolume: delete volume backend response's statuscode: %d, res: %s", response.StatusCode(), string(response.Body()))

	if response.StatusCode() != http.StatusOK {
		if response.StatusCode() == 401 {
			klog.Errorf("DeleteVolume: no permission to delete vol-%s", volume.VolumeId)
		}
		return nil, status.Errorf(codes.Internal, "DeleteVolume: backend has response but not success")
	}

	delete(createdVolumeMap, volume.VolumeId)
	err = os.Remove(volumeMapFilePath)
	if err != nil {
		klog.Warningf("DeleteVolume: can't remove vol-%s mapping file %s for error: %s.", volume.VolumeId, volumeMapFilePath, err)
	}

	delete(deletingVolumeReqMap, volume.GetVolumeId())
	klog.Infof("DeleteVolume: delete vol-%s success.", volume.VolumeId)

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
