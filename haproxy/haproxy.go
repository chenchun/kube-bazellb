package haproxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/golang/glog"
)

type Haproxy struct {
	ConfFile   string
	ConfigChan chan *bytes.Buffer
}

func NewHaproxy() *Haproxy {
	return &Haproxy{
		ConfFile:   "/etc/haproxy/haproxy.cfg",
		ConfigChan: make(chan *bytes.Buffer),
	}
}

func (h *Haproxy) buildConf() error {
	if err := os.MkdirAll(filepath.Dir(h.ConfFile), 0755); err != nil {
		return fmt.Errorf("failed to mkdir for conf %s: %v", h.ConfFile, err)
	}
	if err := ioutil.WriteFile(h.ConfFile, []byte(GetSampleTemplate()), 0644); err != nil {
		return fmt.Errorf("failed to write conf %s: %v", h.ConfFile, err)
	}
	return nil
}

//Start starts haproxy
func (h *Haproxy) Start() error {
	if err := h.buildConf(); err != nil {
		return err
	}
	cmd := exec.Cmd{
		Path:   "/usr/local/sbin/haproxy",
		Args:   []string{"/usr/local/sbin/haproxy", "-f", h.ConfFile, "-D"},
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
	glog.Infof("haproxy started")
	return nil
}

func (h *Haproxy) Run() {
	select {
	case buf := <-h.ConfigChan:
		if err := ioutil.WriteFile(h.ConfFile, buf.Bytes(), 0644); err != nil {
			fmt.Errorf("failed to write conf %s: %v", h.ConfFile, err)
		}
	}
}
