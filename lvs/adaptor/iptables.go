package adaptor

import (
	"strings"

	"github.com/chenchun/kube-bmlb/utils/ipset"
	"github.com/chenchun/kube-bmlb/utils/iptables"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	ipsetName = "bmlb-vip-vport"
	mark      = "0x4000/0x4000"
)

var (
	constRules = []struct {
		position iptables.RulePosition
		table    iptables.Table
		chain    iptables.Chain
		rules    []string
	}{
		//TODO add comment
		//-A POSTROUTING -m mark --mark 0x4000/0x4000 -j MASQUERADE
		{position: iptables.Prepend, table: iptables.TableNAT, chain: "OUTPUT", rules: []string{"-p", "all", "-m", "set", "--match-set", ipsetName, "dst,dst", "-j", "MARK", "--set-xmark", mark}},
		{position: iptables.Prepend, table: iptables.TableNAT, chain: "PREROUTING", rules: []string{"-p", "all", "-m", "set", "--match-set", ipsetName, "dst,dst", "-j", "MARK", "--set-xmark", mark}},
		{position: iptables.Prepend, table: iptables.TableNAT, chain: "POSTROUTING", rules: []string{"-m", "mark", "--mark", mark, "-j", "MASQUERADE"}},
	}
)

// buildIptables builds iptables and ipsets for input services
// serviceMap //protocol port:service, removeOldVS bool
func (a *LVSAdaptor) buildIptables(serviceMap []map[int]*v1.Service, removeOldVS bool) {
	set := &ipset.IPSet{Name: ipsetName, SetType: ipset.HashIPPort}
	if err := a.ipsetHandler.CreateSet(set, true); err != nil {
		glog.Warningf("failed to create ipset %v: %v", set, err)
		return
	}
	for _, rule := range constRules {
		if _, err := a.iptHandler.EnsureRule(rule.position, rule.table, rule.chain, rule.rules...); err != nil {
			glog.Warningf("failed to add iptables rule %s: %v", strings.Join(append([]string{"-t", string(rule.table), string(rule.position), string(rule.chain)}, rule.rules...), " "))
		}
	}
	expectEntries := sets.String{}
	for i := range serviceMap {
		protocol := "tcp"
		if i == 1 {
			protocol = "udp"
		}
		for p := range serviceMap[i] {
			expectEntries.Insert((&ipset.Entry{IP: a.virtualServerAddress.String(), Port: p, Protocol: protocol, SetType: set.SetType}).String())
		}
	}
	existEntries, err := a.ipsetHandler.ListEntries(ipsetName)
	if err != nil {
		glog.Warningf("failed to list ipset entries: %v", err)
		return
	} else {
		if removeOldVS {
			for _, existEntry := range existEntries {
				if !expectEntries.Has(existEntry) {
					if err := a.ipsetHandler.DelEntry(existEntry, ipsetName); err != nil {
						glog.Warningf("failed to del ipset entry %s: %v", existEntry, err)
					}
				} else {
					delete(expectEntries, existEntry)
				}
			}
		}
	}
	for _, expectEntry := range expectEntries.List() {
		if err := a.ipsetHandler.AddEntry(expectEntry, set, true); err != nil {
			glog.Warningf("failed to add ipset entry %s: %v", expectEntry, err)
		}
	}
}
