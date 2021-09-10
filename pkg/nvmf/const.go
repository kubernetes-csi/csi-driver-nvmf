package nvmf

const (
	NVMF_NQN_SIZE = 223
	SYS_NVMF      = "/sys/class/nvmf"
)

// Here erron
const (
	ENOENT = 1 /* No such file or directory */
	EINVAL = 2 /* Invalid argument */
)

const (
	DefaultDriverName        = "csi.nvmf.com"
	DefaultDriverServicePort = "12230"
	DefaultDriverVersion     = "v1.0.0"

	DefaultVolumeMapPath = "/var/lib/nvmf/volumes"
)

type GlobalConfig struct {
	NVMfVolumeMapDir   string
	DriverName         string
	Region             string
	NodeID             string
	Endpoint           string // CSI endpoint
	Version            string
	IsControllerServer bool
	LogLevel           string
}
