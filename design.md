## Design

- A LB defined as a static reachable address - VIP.
  - Built in HA. VIP is backended by at least two nodes
  - Quantify the power of a single LB. The performance differs on different NIC/machine. Report according to a test in reality
  - Controllable traffic adjustment
- LB lives on kubelet machines as daemonset
  - Network and Computing interlace on kubelet
  - LB daemonset joins host network
- LB implementation
  - L4: LVS/haproxy
  - L7 haproxy
- An APP(deployment/stateful set) may become endpoints of multiple LBs
  - A DNS server replaces the position of a single static address

```
   D   N   S        <- A single static domain address
 --||--\\--\\
   LB1  LB2 LBN..   <- Each LB is HA. 
    | \ / |           \_ The num of LBs for a single App is scalable. 
    | / \ |
   Pod  Pod

# VIP of LB flows onto Node2 after Node1 down
Node1 Node2            Node1 Node2
   |          ->             /   
   LB        Node1 down    LB
```

## Controllable traffic adjustment

Full control of the traffic proportion during upgrade

1. 100% -> v1
1. 10% -> v2, 90% -> v1
1. 50% -> v2, 50% -> v1
1. 100% -> v2.

```
     L B
  / |  | \
p1 p2 p3 p4
\ /    \ /
v1     v2

p1 p2: pod based on image:v1, deployment v1
p3 p4: pod based on image:v2, deployment v2

```

## The goal

- inside cluster traffic to services which is in the same cluster
- outside cluster traffic to services in cluster

The first traffic doesn't require creating iptables rules on all nodes like kube-proxy. 
We will implement such things inside container like istio.

For the second traffic, we don't have a bare metal usable load balance. Nodeport service is not a HA solution.

For both of them, we need a controllable traffic adjustment function.