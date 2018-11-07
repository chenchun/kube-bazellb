package api

import (
	"encoding/json"
)

var (
	ANWeight = "v1.bmlb.l4/weight"
)

type Weight map[int]uint

func DecodeL4Weight(weightStr string) (map[int]uint, error) {
	var w Weight
	if err := json.Unmarshal([]byte(weightStr), &w); err != nil {
		return nil, err
	}
	return w, nil
}
