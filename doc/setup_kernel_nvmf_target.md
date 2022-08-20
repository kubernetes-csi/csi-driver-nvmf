This a simple guide to setup a nvmf target

### 1. Modprobe nvmet-tcp
``` bash
modprobe nvmet-tcp
```

### 2. Prepare a storage backend device (file or block)

```bash
#file
truncate --size=20G /tmp/nvmet_test.img

#block is like /dev/nvme1n1
```

### 3.1 Create nvmf subsystem

example subsystem name: nqn.2022-08.org.test-nvmf.example

``` bash
mkdir /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example
echo 1 > /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/attr_allow_any_host
echo 0123456789abcdef > /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/attr_serial
```

### 3.2 Create nvmf namespace

``` bash
mkdir /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/namespaces/1

# file storage backend
echo "/tmp/nvmet_test.img" > /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/namespaces/1/device_path

# block storage backend
echo "/dev/nvme1n1" > /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/namespaces/1/device_path

echo 1 > /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/namespaces/1/enable
```

### 3.3 Create nvmf port

``` bash
mkdir /sys/kernel/config/nvmet/ports/1
echo ipv4 > /sys/kernel/config/nvmet/ports/1/addr_adrfam
echo tcp > /sys/kernel/config/nvmet/ports/1/addr_trtype #target transport type is tcp
echo "192.168.122.18" > /sys/kernel/config/nvmet/ports/1/addr_traddr # target addr is 192.168.122.18
echo 49153 > /sys/kernel/config/nvmet/ports/1/addr_trsvcid # target port is 49153 (rdms should be 4420)
```

### 3.4 Link port and namespace

``` bash
ln -s /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/namespaces/ /sys/kernel/config/nvmet/ports/1/subsystems/nqn.2022-08.org.test-nvmf.example
```

### 3.5 Record the DeviceUUID

``` bash
cat  /sys/kernel/config/nvmet/subsystems/nqn.2022-08.org.test-nvmf.example/namespaces/1/device_uuid
```