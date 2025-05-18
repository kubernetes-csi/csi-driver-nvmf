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
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/utils"
	"k8s.io/klog/v2"
)

type Connector struct {
	VolumeID        string
	TargetNqn       string
	TargetEndpoints []string
	Transport       string
	HostNqn         string
	RetryCount      int32
	CheckInterval   int32
}

func getNvmfConnector(nvmfInfo *nvmfDiskInfo, hostnqn string) *Connector {
	return &Connector{
		VolumeID:        nvmfInfo.VolName,
		TargetNqn:       nvmfInfo.Nqn,
		TargetEndpoints: nvmfInfo.Endpoints,
		Transport:       nvmfInfo.Transport,
		HostNqn:         hostnqn,
		RetryCount:      10, // Default retry count
		CheckInterval:   1,  // Default check interval in seconds
	}
}

// connector provides a struct to hold all of the needed parameters to make nvmf connection

func _connect(argStr string) error {
	file, err := os.OpenFile("/dev/nvme-fabrics", os.O_RDWR, 0666)
	if err != nil {
		klog.Errorf("Connect: open NVMf fabrics error: %v", err)
		return err
	}

	defer file.Close()

	err = utils.WriteStringToFile(file, argStr)
	if err != nil {
		klog.Errorf("Connect: write arg to connect file error: %v", err)
		return err
	}
	// todo: read file to verify
	lines, _ := utils.ReadLinesFromFile(file)
	klog.Infof("Connect: read string %s", lines)
	return nil
}

func _disconnect(sysfs_path string) error {
	file, err := os.OpenFile(sysfs_path, os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	err = utils.WriteStringToFile(file, "1")
	if err != nil {
		klog.Errorf("Disconnect: write 1 to delete_controller error: %v", err)
		return err
	}
	return nil
}

func disconnectSubsysWithHostNqn(nqn, hostnqn, ctrl string) error {
	sysfs_subsysnqn_path := fmt.Sprintf("%s/%s/subsysnqn", SYS_NVMF, ctrl)
	sysfs_hostnqn_path := fmt.Sprintf("%s/%s/hostnqn", SYS_NVMF, ctrl)
	sysfs_del_path := fmt.Sprintf("%s/%s/delete_controller", SYS_NVMF, ctrl)

	file, err := os.Open(sysfs_subsysnqn_path)
	if err != nil {
		klog.Errorf("Disconnect: open file %s err: %v", sysfs_subsysnqn_path, err)
		return &NoControllerError{Nqn: nqn, Hostnqn: hostnqn}
	}
	defer file.Close()

	lines, err := utils.ReadLinesFromFile(file)
	if err != nil {
		klog.Errorf("Disconnect: read file %s err: %v", file.Name(), err)
		return &NoControllerError{Nqn: nqn, Hostnqn: hostnqn}
	}

	if lines[0] != nqn {
		klog.Warningf("Disconnect: not this subsystem, skip")
		return &NoControllerError{Nqn: nqn, Hostnqn: hostnqn}
	}

	file, err = os.Open(sysfs_hostnqn_path)
	if err != nil {
		klog.Errorf("Disconnect: open file %s err: %v", sysfs_hostnqn_path, err)
		return &UnsupportedHostnqnError{Target: sysfs_hostnqn_path}
	}
	defer file.Close()

	lines, err = utils.ReadLinesFromFile(file)
	if err != nil {
		klog.Errorf("Disconnect: read file %s err: %v", file.Name(), err)
		return &NoControllerError{Nqn: nqn, Hostnqn: hostnqn}
	}

	if lines[0] != hostnqn {
		klog.Warningf("Disconnect: not this subsystem, skip")
		return &NoControllerError{Nqn: nqn, Hostnqn: hostnqn}
	}

	err = _disconnect(sysfs_del_path)
	if err != nil {
		klog.Errorf("Disconnect: disconnect error: %s", err)
		return &NoControllerError{Nqn: nqn, Hostnqn: hostnqn}
	}

	return nil
}

func disconnectSubsys(nqn, ctrl string) error {
	sysfs_subsysnqn_path := fmt.Sprintf("%s/%s/subsysnqn", SYS_NVMF, ctrl)
	sysfs_del_path := fmt.Sprintf("%s/%s/delete_controller", SYS_NVMF, ctrl)

	file, err := os.Open(sysfs_subsysnqn_path)
	if err != nil {
		klog.Errorf("Disconnect: open file %s err: %v", sysfs_subsysnqn_path, err)
		return &NoControllerError{Nqn: nqn, Hostnqn: ""}
	}
	defer file.Close()

	lines, err := utils.ReadLinesFromFile(file)
	if err != nil {
		klog.Errorf("Disconnect: read file %s err: %v", file.Name(), err)
		return &NoControllerError{Nqn: nqn, Hostnqn: ""}
	}

	if lines[0] != nqn {
		klog.Warningf("Disconnect: not this subsystem, skip")
		return &NoControllerError{Nqn: nqn, Hostnqn: ""}
	}

	err = _disconnect(sysfs_del_path)
	if err != nil {
		klog.Errorf("Disconnect: disconnect error: %s", err)
		return &NoControllerError{Nqn: nqn, Hostnqn: ""}
	}

	return nil
}

func disconnectByNqn(nqn, hostnqn string) int {
	ret := 0
	if len(nqn) > NVMF_NQN_SIZE {
		klog.Errorf("Disconnect: nqn %s is too long ", nqn)
		return -EINVAL
	}

	// delete hostnqn file
	hostnqnPath := filepath.Join(RUN_NVMF, nqn, b64.StdEncoding.EncodeToString([]byte(hostnqn)))
	os.Remove(hostnqnPath)

	// delete nqn directory if has no hostnqn files
	nqnPath := filepath.Join(RUN_NVMF, nqn)
	hostnqns, err := os.ReadDir(nqnPath)
	if err != nil {
		klog.Errorf("Disconnect: readdir %s err: %v", nqnPath, err)
		return -ENOENT
	}
	if len(hostnqns) <= 0 {
		os.RemoveAll(nqnPath)
	}

	devices, err := os.ReadDir(SYS_NVMF)
	if err != nil {
		klog.Errorf("Disconnect: readdir %s err: %s", SYS_NVMF, err)
		return -ENOENT
	}

	for _, device := range devices {
		if err := disconnectSubsysWithHostNqn(nqn, hostnqn, device.Name()); err != nil {
			if _, ok := err.(*UnsupportedHostnqnError); ok {
				klog.Infof("Fallback because you have no hostnqn supports!")

				// disconnect all controllers if has no hostnqn files
				if len(hostnqns) <= 0 {
					devices, err := os.ReadDir(SYS_NVMF)
					if err != nil {
						klog.Errorf("Disconnect: readdir %s err: %s", SYS_NVMF, err)
						return -ENOENT
					}

					for _, device := range devices {
						if err := disconnectSubsys(nqn, device.Name()); err == nil {
							ret++
						}
					}
				}

				return ret
			}
		} else {
			ret++
		}
	}

	return ret
}

// connect to volume to this node and return devicePath
func (c *Connector) Connect() (string, error) {
	if c.RetryCount < 0 || c.CheckInterval < 0 {
		return "", fmt.Errorf("Invalid RetryCount and CheckInterval combinaitons "+
			"RetryCount: %d, CheckInterval: %d ", c.RetryCount, c.CheckInterval)
	}

	if strings.ToLower(c.Transport) != "tcp" && strings.ToLower(c.Transport) != "rdma" {
		return "", fmt.Errorf("csi transport only support tcp/rdma ")
	}

	// TargetEndpoints is assumed to be populated (via CreateVolume) with multiple "IP:Port" entries
	// Attempt to connect to all endpoints to support multi-path configurations
	for _, endpoint := range c.TargetEndpoints {
		// Split the endpoint into IP and port
		parts := strings.Split(endpoint, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid endpoint format: %s", endpoint)
		}

		ip, port := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if ip == "" || port == "" {
			return "", fmt.Errorf("empty IP or port in endpoint: %s", endpoint)
		}

		baseString := fmt.Sprintf("nqn=%s,transport=%s,traddr=%s,trsvcid=%s,hostnqn=%s", c.TargetNqn, c.Transport, ip, port, c.HostNqn)
		klog.V(4).Infof("Running connect on %s://%s:%s", c.Transport, ip, port)

		// connect to nvmf disk
		err := _connect(baseString)
		if err != nil {
			klog.Errorf("Connect: failed to connect to endpoint %s, error: %v", endpoint, err)
			return "", err
		}
	}
	klog.V(4).Infof("Connect Volume %s success nqn: %s, hostnqn: %s", c.VolumeID, c.TargetNqn, c.HostNqn)

	// Wait for device to be ready (find UUID and check path)
	devicePath, err := findPathWithRetry(c.TargetNqn, c.RetryCount, c.CheckInterval)
	if err != nil {
		klog.Errorf("connect nqn %s error %v, rollback!!!", c.TargetNqn, err)
		ret := disconnectByNqn(c.TargetNqn, c.HostNqn)
		if ret < 0 {
			klog.Errorf("rollback error !!!")
		}
		return "", err
	}

	// create tracking files
	if err := createTrackingFiles(c); err != nil {
		klog.Errorf("create nqn directory %s error %v, rollback!!!", c.TargetNqn, err)
		ret := disconnectByNqn(c.TargetNqn, c.HostNqn)
		if ret < 0 {
			klog.Errorf("rollback error !!!")
		}
		return "", err
	}

	klog.Infof("After connect we're returning devicePath: %s", devicePath)
	return devicePath, nil
}

// we disconnect only by nqn
func (c *Connector) Disconnect() error {
	ret := disconnectByNqn(c.TargetNqn, c.HostNqn)
	if ret < 0 {
		return fmt.Errorf("Disconnect: failed to disconnect by nqn: %s ", c.TargetNqn)
	}

	return nil
}

// createTrackingFiles creates tracking files used by the disconnect process
func createTrackingFiles(c *Connector) error {
	// create nqn directory
	nqnPath := filepath.Join(RUN_NVMF, c.TargetNqn)
	if err := os.MkdirAll(nqnPath, 0750); err != nil {
		return fmt.Errorf("failed to create NQN directory: %v", err)
	}

	// create hostnqn file
	hostnqnPath := filepath.Join(RUN_NVMF, c.TargetNqn, b64.StdEncoding.EncodeToString([]byte(c.HostNqn)))
	file, err := os.Create(hostnqnPath)
	if err != nil {
		klog.Errorf("create hostnqn file %s:%s error %v", c.TargetNqn, c.HostNqn, err)
	}
	defer file.Close()

	return nil
}

// PersistConnector persists the provided Connector to the specified file (ie /var/lib/pfile/myConnector.json)
func persistConnectorFile(c *Connector, filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating nvmf persistence file %s: %s", filePath, err)
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	if err = encoder.Encode(c); err != nil {
		return fmt.Errorf("error encoding connector: %v", err)
	}
	return nil

}

func removeConnectorFile(targetPath string) {
	// todo: here maybe be attack for os.Remove can operate any file, fix?
	if err := os.Remove(targetPath + ".json"); err != nil {
		klog.Errorf("DetachDisk: Can't remove connector file: %s", targetPath)
	}
	if err := os.RemoveAll(targetPath); err != nil {
		klog.Errorf("DetachDisk: failed to remove mount path Error: %v", err)
	}
}

func GetConnectorFromFile(filePath string) (*Connector, error) {
	f, err := os.ReadFile(filePath)
	if err != nil {
		return &Connector{}, err

	}
	data := Connector{}
	err = json.Unmarshal([]byte(f), &data)
	if err != nil {
		return &Connector{}, err
	}

	return &data, nil
}
