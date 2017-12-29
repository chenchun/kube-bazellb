- A LB defined as a static reachable address - VIP.
  - Built in HA. VIP is backended by at least two nodes
  - Quantify the power of a single LB. The performance differs on different NIC/machine. Report according to a test in reality
- LB lives on kubelet machines as daemonset
  - Network and Computing interlace on kubelet
  - LB daemonset joins host network
- LB implementation
  - L4: LVS/haproxy
  - L7 haproxy
- An APP(deployment/stateful set) may become endpoints of multiple LBs
  - A DNS server replaces the position of a single static address
  
```
   DNS
 --||-\\--
   LB  LB     <- each LB is HA 
  / \   | \
Pod Pod Pod Pod
```
