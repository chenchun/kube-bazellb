package lvs

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"net"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/docker/libnetwork/ipvs"
)

// Interface is an injectable interface for running ipvs commands.  Implementations must be goroutine-safe.
type Interface interface {
	// Flush clears all virtual servers in system. return occurred error immediately.
	Flush() error
	// AddVirtualServer creates the specified virtual server.
	AddVirtualServer(*VirtualServer) error
	// UpdateVirtualServer updates an already existing virtual server.  If the virtual server does not exist, return error.
	UpdateVirtualServer(*VirtualServer) error
	// DeleteVirtualServer deletes the specified virtual server.  If the virtual server does not exist, return error.
	DeleteVirtualServer(*VirtualServer) error
	// Given a partial virtual server, GetVirtualServer will return the specified virtual server information in the system.
	GetVirtualServer(*VirtualServer) (*VirtualServer, error)
	// GetVirtualServers lists all virtual servers in the system.
	GetVirtualServers() ([]*VirtualServer, error)
	// AddRealServer creates the specified real server for the specified virtual server.
	AddRealServer(*VirtualServer, *RealServer) error
	// GetRealServers returns all real servers for the specified virtual server.
	GetRealServers(*VirtualServer) ([]*RealServer, error)
	// DeleteRealServer deletes the specified real server from the specified virtual server.
	DeleteRealServer(*VirtualServer, *RealServer) error
}

// VirtualServer is an user-oriented definition of an IPVS virtual server in its entirety.
type VirtualServer struct {
	Address   net.IP
	Protocol  string
	Port      uint16
	Scheduler string
	Flags     ServiceFlags
	Timeout   uint32
}

// ServiceFlags is used to specify session affinity, ip hash etc.
type ServiceFlags uint32

const (
	// FlagPersistent specify IPVS service session affinity
	FlagPersistent = 0x1
	// FlagHashed specify IPVS service hash flag
	FlagHashed = 0x2
)

// Equal check the equality of virtual server.
// We don't use struct == since it doesn't work because of slice.
func (svc *VirtualServer) Equal(other *VirtualServer) bool {
	return svc.Address.Equal(other.Address) &&
		svc.Protocol == other.Protocol &&
		svc.Port == other.Port &&
		svc.Scheduler == other.Scheduler &&
		svc.Flags == other.Flags &&
		svc.Timeout == other.Timeout
}

func (svc *VirtualServer) String() string {
	return net.JoinHostPort(svc.Address.String(), strconv.Itoa(int(svc.Port))) + "/" + svc.Protocol
}

// RealServer is an user-oriented definition of an IPVS real server in its entirety.
type RealServer struct {
	Address net.IP
	Port    uint16
	Weight  int
}

func (rs *RealServer) String() string {
	return net.JoinHostPort(rs.Address.String(), strconv.Itoa(int(rs.Port)))
}

// Equal check the equality of real server.
// We don't use struct == since it doesn't work because of slice.
func (rs *RealServer) Equal(other *RealServer) bool {
	return rs.Address.Equal(other.Address) &&
		rs.Port == other.Port &&
		rs.Weight == other.Weight
}

// runner implements Interface.
type runner struct {
	ipvsHandle *ipvs.Handle
}

// New returns a new Interface which will call ipvs APIs.
func New() Interface {
	ihandle, err := ipvs.New("")
	if err != nil {
		glog.Errorf("IPVS interface can't be initialized, error: %v", err)
		return nil
	}
	return &runner{
		ipvsHandle: ihandle,
	}
}

// AddVirtualServer is part of Interface.
func (runner *runner) AddVirtualServer(vs *VirtualServer) error {
	eSvc, err := toBackendService(vs)
	if err != nil {
		return err
	}
	glog.V(5).Infof("AddVirtualServer vs %s", vs.String())
	return runner.ipvsHandle.NewService(eSvc)
}

// UpdateVirtualServer is part of Interface.
func (runner *runner) UpdateVirtualServer(vs *VirtualServer) error {
	bSvc, err := toBackendService(vs)
	if err != nil {
		return err
	}
	return runner.ipvsHandle.UpdateService(bSvc)
}

// DeleteVirtualServer is part of Interface.
func (runner *runner) DeleteVirtualServer(vs *VirtualServer) error {
	bSvc, err := toBackendService(vs)
	if err != nil {
		return err
	}
	glog.V(5).Infof("DeleteVirtualServer vs %s", vs.String())
	return runner.ipvsHandle.DelService(bSvc)
}

// GetVirtualServer is part of Interface.
func (runner *runner) GetVirtualServer(vs *VirtualServer) (*VirtualServer, error) {
	bSvc, err := toBackendService(vs)
	if err != nil {
		return nil, err
	}
	ipvsService, err := runner.ipvsHandle.GetService(bSvc)
	if err != nil {
		return nil, err
	}
	virtualServer, err := toVirtualServer(ipvsService)
	if err != nil {
		return nil, err
	}
	return virtualServer, nil
}

// GetVirtualServers is part of Interface.
func (runner *runner) GetVirtualServers() ([]*VirtualServer, error) {
	ipvsServices, err := runner.ipvsHandle.GetServices()
	if err != nil {
		return nil, err
	}
	vss := make([]*VirtualServer, 0)
	for _, ipvsService := range ipvsServices {
		vs, err := toVirtualServer(ipvsService)
		if err != nil {
			return nil, err
		}
		vss = append(vss, vs)
	}
	return vss, nil
}

// Flush is part of Interface.  Currently we delete IPVS services one by one
func (runner *runner) Flush() error {
	return runner.ipvsHandle.Flush()
}

// AddRealServer is part of Interface.
func (runner *runner) AddRealServer(vs *VirtualServer, rs *RealServer) error {
	bSvc, err := toBackendService(vs)
	if err != nil {
		return err
	}
	bDst, err := toBackendDestination(rs)
	if err != nil {
		return err
	}
	glog.V(5).Infof("AddRealServer vs %s rs %s", vs.String(), rs.String())
	return runner.ipvsHandle.NewDestination(bSvc, bDst)
}

// DeleteRealServer is part of Interface.
func (runner *runner) DeleteRealServer(vs *VirtualServer, rs *RealServer) error {
	bSvc, err := toBackendService(vs)
	if err != nil {
		return err
	}
	bDst, err := toBackendDestination(rs)
	if err != nil {
		return err
	}
	glog.V(5).Infof("DeleteRealServer vs %s rs %s", vs.String(), rs.String())
	return runner.ipvsHandle.DelDestination(bSvc, bDst)
}

// GetRealServers is part of Interface.
func (runner *runner) GetRealServers(vs *VirtualServer) ([]*RealServer, error) {
	bSvc, err := toBackendService(vs)
	if err != nil {
		return nil, err
	}
	bDestinations, err := runner.ipvsHandle.GetDestinations(bSvc)
	if err != nil {
		return nil, err
	}
	realServers := make([]*RealServer, 0)
	for _, dest := range bDestinations {
		dst, err := toRealServer(dest)
		// TODO: aggregate errors?
		if err != nil {
			return nil, err
		}
		realServers = append(realServers, dst)
	}
	return realServers, nil
}

// toVirtualServer converts an IPVS service representation to the equivalent virtual server structure.
func toVirtualServer(svc *ipvs.Service) (*VirtualServer, error) {
	if svc == nil {
		return nil, errors.New("ipvs svc should not be empty")
	}
	vs := &VirtualServer{
		Address:   svc.Address,
		Port:      svc.Port,
		Scheduler: svc.SchedName,
		Protocol:  protocolNumbeToString(ProtoType(svc.Protocol)),
		Timeout:   svc.Timeout,
	}

	// Test Flags >= 0x2, valid Flags ranges [0x2, 0x3]
	if svc.Flags&FlagHashed == 0 {
		return nil, fmt.Errorf("Flags of successfully created IPVS service should be >= %d since every service is hashed into the service table", FlagHashed)
	}
	// Sub Flags to 0x2
	// 011 -> 001, 010 -> 000
	vs.Flags = ServiceFlags(svc.Flags &^ uint32(FlagHashed))

	if vs.Address == nil {
		if svc.AddressFamily == syscall.AF_INET {
			vs.Address = net.IPv4zero
		} else {
			vs.Address = net.IPv6zero
		}
	}
	return vs, nil
}

// toRealServer converts an IPVS destination representation to the equivalent real server structure.
func toRealServer(dst *ipvs.Destination) (*RealServer, error) {
	if dst == nil {
		return nil, errors.New("ipvs destination should not be empty")
	}
	return &RealServer{
		Address: dst.Address,
		Port:    dst.Port,
		Weight:  dst.Weight,
	}, nil
}

// toBackendService converts an IPVS real server representation to the equivalent "backend" service structure.
func toBackendService(vs *VirtualServer) (*ipvs.Service, error) {
	if vs == nil {
		return nil, errors.New("virtual server should not be empty")
	}
	bakSvc := &ipvs.Service{
		Address:   vs.Address,
		Protocol:  stringToProtocolNumber(vs.Protocol),
		Port:      vs.Port,
		SchedName: vs.Scheduler,
		Flags:     uint32(vs.Flags),
		Timeout:   vs.Timeout,
	}

	if ip4 := vs.Address.To4(); ip4 != nil {
		bakSvc.AddressFamily = syscall.AF_INET
		bakSvc.Netmask = 0xffffffff
	} else {
		bakSvc.AddressFamily = syscall.AF_INET6
		bakSvc.Netmask = 128
	}
	return bakSvc, nil
}

// toBackendDestination converts an IPVS real server representation to the equivalent "backend" destination structure.
func toBackendDestination(rs *RealServer) (*ipvs.Destination, error) {
	if rs == nil {
		return nil, errors.New("real server should not be empty")
	}
	return &ipvs.Destination{
		Address: rs.Address,
		Port:    rs.Port,
		Weight:  rs.Weight,
	}, nil
}

// stringToProtocolNumber returns the protocol value for the given name
func stringToProtocolNumber(protocol string) uint16 {
	switch strings.ToLower(protocol) {
	case "tcp":
		return uint16(syscall.IPPROTO_TCP)
	case "udp":
		return uint16(syscall.IPPROTO_UDP)
	}
	return uint16(0)
}

// protocolNumbeToString returns the name for the given protocol value.
func protocolNumbeToString(proto ProtoType) string {
	switch proto {
	case syscall.IPPROTO_TCP:
		return "TCP"
	case syscall.IPPROTO_UDP:
		return "UDP"
	}
	return ""
}

// ProtoType is IPVS service protocol type
type ProtoType uint16

func Dump(iface Interface) (string, error) {
	buf := bytes.NewBuffer(nil)
	vss, err := iface.GetVirtualServers()
	if err != nil {
		return "", err
	}
	vsMap := map[string]*VirtualServer{}
	vsStrs := make([]string, len(vss))
	for i, vs := range vss {
		vsMap[vs.String()] = vs
		vsStrs[i] = vs.String()
	}
	sort.Strings(vsStrs)
	for i := range vsStrs {
		vsStr := vsStrs[i]
		vs := vsMap[vsStr]
		buf.WriteString(vs.String())
		buf.WriteByte('\n')
		rss, err := iface.GetRealServers(vs)
		if err != nil {
			return "", err
		}
		rsStrs := make([]string, len(rss))
		for i, rs := range rss {
			rsStrs[i] = rs.String()
		}
		sort.Strings(rsStrs)
		for _, str := range rsStrs {
			buf.WriteString(fmt.Sprintf("  -> %s", str))
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	if buf.Len() > 0 {
		buf.Truncate(buf.Len() - 1)
	}
	return buf.String(), nil
}
