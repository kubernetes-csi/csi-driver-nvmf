# CSI NVMf driver

## Overview

This is a repository for [NVMe-oF](https://en.wikipedia.org/wiki/NVM_Express#NVMe-oF) [CSI](https://kubernetes-csi.github.io/docs/) Driver. 
Currently it implements bare minimum of th [CSI spec](https://github.com/container-storage-interface/spec).

## Requirements

The CSI NVMf driver requires initiator and target kernel versions to be Linux kernel 5.0 or newer.
Before using this csi driver, you should create a NVMf remote disk on the target side and record traddr/trport/trtype/nqn/deviceuuid.

## Modprobe Nvmf mod on Initiator/Target

```
# when use TCP as transport
$ modprobe nvme-tcp
```

```
# when use RDMA as transport
$ modprobe nvme-rdma
```

## Test NVMf driver using csc
Get csc tool from https://github.com/rexray/gocsi/tree/master/csc
```
$ go get github.com/rexray/gocsi/csc
```

### 1. Complile NVMf driver
```
$ make
```

### 2. Start NVMf driver

```
$ ./output/nvmfplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode
```

### 3.1 Get plugin info
```
$ csc identity plugin-info --endpoint tcp://127.0.0.1:10000
"csi.nvmf.com" "v1.0.0"
```
### 3.2 NodePublish a volume
```
$ export TargetTrAddr="NVMf Target Server IP (Ex: 192.168.122.18)"
$ export TargetTrPort="NVMf Target Server Ip Port (Ex: 49153)"
$ export TargetTrType="NVMf Target Type (Ex: tcp | rdma)"
$ export DeviceUUID="NVMf Target Device UUID (Ex: 58668891-c3e4-45d0-b90e-824525c16080)"
$ export NQN="NVMf Target NQN"
$ csc node publish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nvmf --attrib targetTrAddr=$TargetTrAddr
                   --attrib targetTrPort=$TargetTrPort --attrib targetTrType=$TargetTrType
                   --attrib deviceUUID=$DeviceUUID --attrib nqn=$NQN nvmftestvol
nvmftestvol
```
You can find a new disk on /mnt/nvmf

### 3.3 NodeUnpublish a volume
```
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/nvmf nvmftestvol
nvmftestvol
```

## Test NVMf driver in kubernetes cluster
> TODO: support dynamic provision.

### 1. Docker Build image
```
$ make container
```

### 2.1 Load Driver
```
$ kubectl create -f deploy/kubernetes/
```
### 2.2 Unload Driver
```
$ kubectl delete -f deploy/kubenetes/
```

### 3.1 Create Storage Class(Dynamic Provisioning) 
> NotSupport for controller not ready
- Create
```
$ kubectl create -f examples/kubernetes/example/storageclass.yaml
```
- Check
```
$ kubectl get sc
```

### 3.2 Create PV(Static Provisioning)
- Create
```
$ kubectl create -f examples/kubernetes/example/pv.yaml
```
- Check
```
$ kubectl get pv
```
### 4. Create Nginx Container
- Create Deployment
```
$ kubectl create -f examples/kubernetes/example/nginx.yaml
```
- Check
```
$ kubectl exec -it nginx-451df123421 /bin/bash
$ lsblk
```

## Community,discussion,contribution,and support

You can reach the maintainers of this project at:
- [Slack](http://slack.k8s.io/)
- [Mailing List](https://groups.google.com/forum/#!forum/kubernetes-dev)

## Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).