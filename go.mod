module github.com/kubernetes-csi/csi-driver-nvmf

go 1.14

require (
	github.com/container-storage-interface/spec v1.5.0
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/golang/glog v1.0.0
	github.com/kubernetes-csi/csi-lib-utils v0.11.0
	github.com/sirupsen/logrus v1.8.1
	golang.org/x/net v0.0.0-20220421235706-1d1ef9303861
	golang.org/x/sys v0.0.0-20220422013727-9388b58f7150 // indirect
	google.golang.org/genproto v0.0.0-20220422154200-b37d22cd5731 // indirect
	google.golang.org/grpc v1.46.0
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.60.1
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
)
