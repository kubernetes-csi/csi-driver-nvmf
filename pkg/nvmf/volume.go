package nvmf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/client"
)

//todo: add allowHostNqn request para
type CreateVolumeRequest struct {
	Name     string
	SizeByte int64
	// AllowHostNqn string
}

type DeleteVolumeRequest struct {
	VolumeId string
}

func ParseCreateVolumeParameters(parameters map[string]string) (volReq *CreateVolumeRequest, err error) {
	//todo: need more parameters for nvmf

	return volReq, nil
}

func GetVolumeContext(volRes *client.CreateVolumeResponse) (map[string]string, error) {
	//todo: volume context parse failed?
	volContext := make(map[string]string)

	volContext[TargetTrAddr] = volRes.TargetConfig.Ports[0].Addr.TrAddr
	volContext[TargetTrPort] = volRes.TargetConfig.Ports[0].Addr.TrSvcID
	volContext[TargetTrType] = volRes.TargetConfig.Ports[0].Addr.TrAddr
	volContext[DeviceUUID] = volRes.TargetConfig.Subsystems[0].Namespaces[0].Device.UUID
	volContext[NQN] = volRes.TargetConfig.Subsystems[0].NQN

	return volContext, nil
}

func PersistVolumeInfo(v *csi.Volume, filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating nvme volume persistence file %s: %v", filePath, err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	if err = encoder.Encode(v); err != nil {
		return fmt.Errorf("error encoding volume: %v", err)
	}
	return nil
}

func GetVolumeInfoFromFile(filePath string) (*csi.Volume, error) {
	f, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	data := csi.Volume{}
	err = json.Unmarshal([]byte(f), &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
