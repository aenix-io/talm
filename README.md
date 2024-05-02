# Talm

Just like Helm, but for Talos Linux

## Getting Started

Create new project
```bash
mkdir newcluster
cd newcluster
talm init
```

Boot Talos Linux node, let's say it has address `1.2.3.4`

Gather node information:
```bash
talm -n 1.2.3.4 -e 1.2.3.4 template templates/controlplane.yaml -i > templates/node1.yaml
```

Edit `templates/node1.yaml` file:
```yaml
machine:
    network:
        # -- Discovered interfaces:
        # enx9c6b0047066c:
        #   name: enp193s0f0
        #   mac:9c:6b:00:47:06:6c
        #   bus:0000:c1:00.0
        #   driver:bnxt_en
        #   vendor: Broadcom Inc. and subsidiaries
        #   product: BCM57414 NetXtreme-E 10Gb/25Gb RDMA Ethernet Controller)
        # enx9c6b0047066d:
        #   name: enp193s0f1
        #   mac:9c:6b:00:47:06:6d
        #   bus:0000:c1:00.1
        #   driver:bnxt_en
        #   vendor: Broadcom Inc. and subsidiaries
        #   product: BCM57414 NetXtreme-E 10Gb/25Gb RDMA Ethernet Controller)
        interfaces:
            - interface: enx9c6b0047066c
              addresses:
                - 1.2.3.4/26
              routes:
                - network: 0.0.0.0/0
                  gateway: 1.2.3.1
        nameservers:
            - 8.8.8.8
            - 8.8.4.4
    install:
        # -- Discovered disks:
        # /dev/nvme0n1:
        #    model: SAMSUNG MZQL21T9HCJR-00A07
        #    serial: S64GNE0RB00153
        #    wwid: eui.3634473052b001530025384500000001
        #    size: 1.75 TB
        # /dev/nvme1n1:
        #    model: SAMSUNG MZQL21T9HCJR-00A07
        #    serial: S64GNE0R811820
        #    wwid: eui.36344730528118200025384500000001
        #    size: 1.75 TB
        disk: /dev/nvme0n1
    type: controlplane
cluster:
    clusterName: talm
    controlPlane:
        endpoint: https://192.168.0.1:6443
```

Show resulted config:
```bash
talm -n 1.2.3.4 -e 1.2.3.4 template templates/node1.yaml -i --full
```

Apply config:
```bash
talm -n 1.2.3.4 -e 1.2.3.4 apply templates/controlplane.yaml -i
```
