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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type driver struct {
	name    string
	nodeId  string
	version string

	region       string
	volumeMapDir string

	idServer         *IdentityServer
	nodeServer       *NodeServer
	controllerServer *ControllerServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

// NewDriver create the identity/node
func NewDriver(conf *GlobalConfig) *driver {
	if conf.DriverName == "" {
		klog.Fatalf("driverName not been specified")
		return nil
	}

	klog.Infof("Driver: %v version: %v", conf.DriverName, conf.Version)
	return &driver{
		name:         conf.DriverName,
		version:      conf.Version,
		nodeId:       conf.NodeID,
		region:       conf.Region,
		volumeMapDir: conf.NVMfVolumeMapDir,
	}
}

func (d *driver) Run(conf *GlobalConfig) {
	d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	})
	d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	})

	d.idServer = NewIdentityServer(d)
	d.nodeServer = NewNodeServer(d)
	if conf.IsControllerServer {
		d.controllerServer = NewControllerServer(d)
	}

	klog.Infof("Starting csi-plugin Driver: %v", d.name)
	s := NewNonBlockingGRPCServer()
	s.Start(conf.Endpoint, d.idServer, d.controllerServer, d.nodeServer)
	s.Wait()
}

func (d *driver) AddVolumeCapabilityAccessModes(caps []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var cap []*csi.VolumeCapability_AccessMode
	for _, c := range caps {
		klog.Infof("Enabling volume access mode: %v", c.String())
		cap = append(cap, &csi.VolumeCapability_AccessMode{Mode: c})
	}
	d.cap = cap
	return cap
}

func (d *driver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability

	for _, c := range cl {
		klog.Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: c,
				},
			},
		})
	}

	d.cscap = csc
}

func (d *driver) ValidateControllerServiceRequest(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range d.cscap {
		if c == cap.GetRpc().GetType() {
			return nil
		}
	}
	return status.Error(codes.InvalidArgument, c.String())
}
