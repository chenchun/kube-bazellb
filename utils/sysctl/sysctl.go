package sysctl

import (
	"github.com/golang/glog"
	"io/ioutil"
	"path"
	"strconv"
	"strings"
)

const (
	sysctlBase = "/proc/sys"
)

// GetSysctl returns the value for the specified sysctl setting
func GetSysctl(sysctl string) (int, error) {
	data, err := ioutil.ReadFile(path.Join(sysctlBase, sysctl))
	if err != nil {
		return -1, err
	}
	val, err := strconv.Atoi(strings.Trim(string(data), " \n"))
	if err != nil {
		return -1, err
	}
	return val, nil
}

// SetSysctl modifies the specified sysctl flag to the new value
func SetSysctl(sysctl string, newVal int) error {
	return ioutil.WriteFile(path.Join(sysctlBase, sysctl), []byte(strconv.Itoa(newVal)), 0640)
}

// EnsureSysctl ensures the specified sysctl flag to the new value if not equal old value
func EnsureSysctl(sysctl string, newVal int) error {
	oldVal, err := GetSysctl(sysctl)
	if err != nil {
		return err
	}
	if oldVal != newVal {
		if err := SetSysctl(sysctl, newVal); err != nil {
			return err
		}
		glog.Infof("Changed sysctl %s from %d to %d", sysctl, oldVal, newVal)
	}
	return nil
}
