package api

import (
	"strconv"
	"strings"
)

var (
	ANNOTATION_KEY_PORT = "v1.bmlb.l4/port"
)

func DecodeL4Ports(portStr string) []int {
	var ports []int
	parts := strings.Split(portStr, ",")
	for j := range parts {
		p, err := strconv.Atoi(parts[j])
		if err != nil {
			continue
		}
		ports = append(ports, p)
	}
	return ports
}

func EncodeL4Ports(ports []string) string {
	return strings.Join(ports, ",")
}
