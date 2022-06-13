package client

type CreateVolumeResponse struct {
	ID            string        `json:"ID,omitempty"`
	CapacityBytes uint64        `json:"capacityBytes,omitempty"`
	TargetType    string        `json:"targetType,omitempty"`
	TargetConfig  *TargetConfig `json:"targetConfig,omitempty"`
}

type TargetConfig struct {
	Hosts      []*Host      `json:"hosts,omitempty"`
	Ports      []*Port      `json:"ports,omitempty"`
	Subsystems []*Subsystem `json:"subsystems,omitempty"`
}

type Host struct {
	NQN string `json:"NQN,omitempty"`
}

type Port struct {
	Addr       *Addr    `json:"addr,omitempty"`
	PortID     uint64   `json:"portID,omitempty"`
	Subsystems []string `json:"subsystems,omitempty"`
}

type Addr struct {
	AdrFam  string `json:"adrFam,omitempty"`
	TrAddr  string `json:"trAddr,omitempty"`
	TrSvcID string `json:"trSvcID,omitempty"`
	TrType  string `json:"trType,omitempty"`
}

type Subsystem struct {
	AllowedHosts []string     `json:"AllowedHosts,omitempty"`
	Attr         *Attr        `json:"attr,omitempty"`
	Namespaces   []*Namespace `json:"namespaces,omitempty"`
	NQN          string       `json:"NQN,omitempty"`
}

type Attr struct {
	AllowAnyHost string `json:"allowAnyHost,omitempty"`
	Serial       string `json:"serial,omitempty"`
}

type Namespace struct {
	Device *Device `json:"device,omitempty"`
	Enable uint32  `json:"enable,omitempty"`
	NSID   uint64  `json:"NSID,omitempty"`
}

type Device struct {
	NGUID string `json:"NGUID,omitempty"`
	Path  string `json:"path,omitempty"`
	UUID  string `json:"UUID,omitempty"`
}
