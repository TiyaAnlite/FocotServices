package main

import "encoding/json"

type Config struct {
	Global struct {
		BUVID   string            `json:"buvid" yaml:"BUVID"`
		UID     uint64            `json:"uid" yaml:"UID"`
		Cookie  string            `json:"cookie" yaml:"Cookie"`
		UA      *string           `json:"ua" yaml:"ua"`
		Headers map[string]string `json:"headers" yaml:"headers"`
	} `json:"global" yaml:"global"` // Global account config
	Provider []*RoomProviderConfig `json:"provider" yaml:"provider"`
}

type RoomProviderConfig struct {
	Type string `json:"type" yaml:"type"`
	json.RawMessage
}
