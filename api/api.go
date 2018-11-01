package api

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

var (
	ANStatusBindedPort = "v1.status.bmlb.l4/bindedPort"
	ANWeight           = "v1.bmlb.l4/weight"
)

// tcp, udp ports
func DecodeL4Ports(portStr string) []map[int32]int32 {
	ports := []map[int32]int32{{}, {}}
	protols := strings.Split(portStr, ";")
	for i, proto := range protols {
		if i == 2 {
			break
		}
		parts := strings.Split(proto, ",")
		for j := range parts {
			p, err := strconv.Atoi(parts[j])
			if err != nil {
				continue
			}
			ports[i][int32(p)] = int32(p)
		}
	}
	return ports
}

func EncodeL4Ports(protolPorts []map[int32]int32) string {
	buf := bytes.NewBuffer(nil)
	for i := range protolPorts {
		if i == 1 {
			buf.WriteString(";")
		} else if i == 2 {
			break
		}
		for _, port := range protolPorts[i] {
			buf.WriteString(strconv.Itoa(int(port)) + ",")
		}
		if len(protolPorts[i]) > 0 {
			buf.Truncate(len(buf.Bytes()) - 1)
		}
	}
	if len(protolPorts) == 0 {
		buf.WriteString(";")
	}
	return string(buf.Bytes())
}

type Weight map[int]uint

func DecodeL4Weight(weightStr string) (map[int]uint, error) {
	var w Weight
	if err := json.Unmarshal([]byte(weightStr), &w); err != nil {
		return nil, err
	}
	return w, nil
}
