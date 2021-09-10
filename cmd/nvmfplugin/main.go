package main

import (
	"csi-driver-nvmf/pkg/nvmf"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"sync"
	"time"
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
}

func main() {
	flag.Parse()
	flag.CommandLine.Parse([]string{})
	cmd := &cobra.Command{
		Use:"NVMf",
		Short: "CSI based NVMf driver",
		Run: func(cmd *cobra.Command, args []string) {
			handle()
		},
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func(endpoint string) {
		defer wg.Done()
		driver := nvmf.NewDriver(&conf)
		driver.Run(&conf)
	}(conf.DriverName)

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
