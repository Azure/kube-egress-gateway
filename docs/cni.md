# CNI

## Design

### Dependencies

wireguard kernel module should be loaded before cni is invoked. This can be done by executing `modprobe wireguard` in the host.
CNI daemon which is responsible for watching Gateway Config and creating Pod Endnpoint Config should be deployed on every node.

### Nic

Nic is created in init namespace and moved to container ns
This nic is attached as secondary nic so this plugin should be used with multus / danm /genie meta cni plugin
### IPAM

ip address is the same as the ipv6 one in eth0.

### Routing

This nic will be the default route for the pod.
But for pod cidr, node cidr and service cidr, we will use the default nic instead.

### Configurations

#### keep-alive

configured on each node, default to true

#### preshared-key

To be discussed.

#### sample cni config
```json
{
    "cniVersion": "1.0.0",
    "name": "mynet",
    "plugins": [
      {
        "type": "kube-egress-cni",
        "ipam": {
          "type": "kube-egress-cni-ipam"
        }
      }
    ]
}
```

### Data Flow

+ parse CNI config and get node cidr, service cidr and pod cidr
+ get k8s metadata from cni args (environment)
+ generates keypairs 
+ exchange public keys with cni daemon and get peer ip and keypairs
+ configures wireguard interface and routes

### Deployment

cni should be deployed by cni daemon

## Reference

+ Wireguard implementation details: [Routing & Network Namespace Integration](https://www.wireguard.com/netns/)
+ [whereabouts](https://github.com/k8snetworkplumbingwg/whereabouts/blob/master/doc/extended-configuration.md)
