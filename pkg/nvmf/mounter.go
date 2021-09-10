package nvmf

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

type nvmfDiskMounter struct {
	*nvmfDiskInfo
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         exec.Interface
	targetPath   string
	connector    *Connector
}

type nvmfDiskUnMounter struct {
	mounter mount.Interface
	exec    exec.Interface
}

func getNVMfDiskMounter(nvmfInfo *nvmfDiskInfo, req *csi.NodePublishVolumeRequest) *nvmfDiskMounter {
	readOnly := req.GetReadonly()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()

	return &nvmfDiskMounter{
		nvmfDiskInfo : nvmfInfo,
		readOnly: readOnly,
		fsType: fsType,
		mountOptions: mountOptions,
		mounter:    &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: exec.New()},
		exec:       exec.New(),
		targetPath: req.GetTargetPath(),
		connector:  getConnector(nvmfInfo),
	}
}

func getNVMfDiskUnMounter(req *csi.NodeUnpublishVolumeRequest) *nvmfDiskUnMounter {
	return &nvmfDiskUnMounter{
		mounter: mount.New(""),
		exec:    exec.New(),
	}
}
