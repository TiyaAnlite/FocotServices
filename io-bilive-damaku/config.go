package main

import (
	"encoding/json"
	"time"
)

type Config struct {
	Global struct {
		Prefix  string            `json:"prefix" yaml:"prefix"`
		BUVID   string            `json:"buvid" yaml:"BUVID"`
		UID     uint64            `json:"uid" yaml:"UID"`
		Cookie  string            `json:"cookie" yaml:"Cookie"`
		UA      *string           `json:"ua" yaml:"ua"`
		Headers map[string]string `json:"headers" yaml:"headers"`
	} `json:"global" yaml:"global"` // Global account config
	Provider   []*RoomProviderConfig `json:"provider" yaml:"provider"`
	Controller ControllerConfig      `json:"controller" yaml:"controller"`
}

type RoomProviderConfig struct {
	Type string `json:"type" yaml:"type"`
	json.RawMessage
}

type ControllerConfig struct {
	DuplicateWindow time.Duration `json:"duplicate_window" yaml:"duplicate_window"`
}

func NewConfig() *Config {
	return &Config{
		Controller: ControllerConfig{
			DuplicateWindow: time.Minute * 10,
		},
	}
}
