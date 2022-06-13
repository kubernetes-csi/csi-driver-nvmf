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

package main

import (
	"flag"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-csi/csi-driver-nvmf/pkg/nvmf"
)

var (
	conf nvmf.GlobalConfig
)

func init() {
	flag.StringVar(&conf.Endpoint, "endpoint", "unix://csi/csi.sock", "CSI endpoint")
	flag.StringVar(&conf.NodeID, "nodeid", "CSINode", "node id")
	flag.BoolVar(&conf.IsControllerServer, "IsControllerServer", false, "Only run as controller service")
	flag.StringVar(&conf.DriverName, "drivername", nvmf.DefaultDriverName, "CSI Driver")
	flag.StringVar(&conf.Region, "region", "test_region", "Region")
	flag.StringVar(&conf.Version, "version", nvmf.DefaultDriverVersion, "Version")

	flag.StringVar(&conf.NVMfVolumeMapDir, "nvmfVolumeMapDir", nvmf.DefaultVolumeMapPath, "Persistent volume")
	flag.StringVar(&conf.NVMfBackendEndpoint, "nvmfBackendEndpoint", nvmf.DefaultBackendEndpoint, "NVMf Volume backend controller")
}

func main() {
	flag.Parse()
	flag.CommandLine.Parse([]string{})
	runDriver()
}

func runDriver() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		driver := nvmf.NewDriver(&conf)
		driver.Run(&conf)
	}()

	servicePort := os.Getenv("SERVICE_PORT")
	if len(servicePort) == 0 || servicePort == "" {
		servicePort = nvmf.DefaultDriverServicePort
	}

	glog.Info("CSI is running")
	server := &http.Server{Addr: ":" + servicePort}
	http.HandleFunc("/healthz", healthHandler)

	if err := server.ListenAndServe(); err != nil {
		glog.Fatalf("Service health port listen and serve err : %s", err.Error())
	}
	wg.Wait()
	os.Exit(0)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	time := time.Now()
	message := "Csi is OK, time:" + time.String()
	w.Write([]byte(message))
}
