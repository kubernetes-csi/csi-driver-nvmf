package nvmf

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
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
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
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

	return
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
	return status.Error(codes.InvalidArgument, fmt.Sprintf("%s", c))
}
