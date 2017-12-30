package main

import (
	"math/rand"
	"time"

	"github.com/chenchun/kube-bmlb/server/bmlb"
	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	s := bmlb.NewServer()
	s.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()
	s.Start()
}
