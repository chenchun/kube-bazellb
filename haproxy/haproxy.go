package haproxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

type Haproxy struct {
	ConfigChan chan *bytes.Buffer
	confFile   string
	pidFile    string
	cmdPath    string
	lastConf   []byte
}

func NewHaproxy() *Haproxy {
	return &Haproxy{
		ConfigChan: make(chan *bytes.Buffer),
		confFile:   "/etc/haproxy/haproxy.cfg",
		pidFile:    "/var/run/haproxy.pid",
		cmdPath:    "/usr/local/sbin/haproxy",
	}
}

func (h *Haproxy) buildConf(data []byte) error {
	h.lastConf = data
	tmpFile := h.confFile + ".tmp"
	if err := os.MkdirAll(filepath.Dir(h.confFile), 0755); err != nil {
		return fmt.Errorf("failed to mkdir for conf %s: %v", h.confFile, err)
	}
	if err := ioutil.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write conf %s: %v", h.confFile, err)
	}
	if status, err := h.checkConfigs(tmpFile); err != nil {
		return fmt.Errorf("haproxy config file is invalid, %s, err %v", string(status), err)
	}
	if err := os.Rename(tmpFile, h.confFile); err != nil {
		return fmt.Errorf("can't rename %s to %s", tmpFile, h.confFile)
	}
	return nil
}

func (h *Haproxy) readPids() (pids []string) {
	data, err := ioutil.ReadFile(h.pidFile)
	if err != nil && !os.IsNotExist(err) {
		glog.Warningf("can't read from pid file %s: %v", h.pidFile, err)
		return
	}
	strs := strings.Split(string(data), "\n")
	for i := range strs {
		if strs[i] == "" {
			continue
		}
		_, err := strconv.Atoi(strs[i])
		if err != nil {
			glog.Warningf("can't parse pid from %q: %v", strs[i], err)
		}
		pids = append(pids, strs[i])
	}
	return
}

func (h *Haproxy) checkConfigs(file string) ([]byte, error) {
	cmd := exec.Cmd{
		Path: h.cmdPath,
		Args: []string{h.cmdPath, "-f", file, "-c"},
	}
	return cmd.CombinedOutput()
}

// Restart gracefully restarts haproxy by starting new haproxy with -sf [oldpids ...] option
// refer: https://www.haproxy.org/download/1.7/doc/management.txt (4. Stopping and restarting HAProxy)
func (h *Haproxy) restart() error {
	cmd := exec.Cmd{
		Path:   h.cmdPath,
		Args:   append([]string{h.cmdPath, "-f", h.confFile, "-D", "-p", h.pidFile, "-sf"}, h.readPids()...),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	glog.Infof("starting haproxy...")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start haproxy: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to wait haproxy: %v", err)
	}
	glog.Infof("haproxy restarted")
	return nil
}

func (h *Haproxy) Run() {
	for {
		select {
		case buf := <-h.ConfigChan:
			data := buf.Bytes()
			if bytes.Equal(h.lastConf, data) {
				glog.V(4).Info("haproxy config unchanged, abort syncing")
				break
			}
			if err := h.buildConf(data); err != nil {
				glog.Warning(err)
				break
			}
			if err := h.restart(); err != nil {
				glog.Warningf("haproxy fails to restart: %v", err)
			}
		}
	}
}
