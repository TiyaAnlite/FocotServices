package main

import (
	"context"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/TiyaAnlite/FocotServicesCommon/dbx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"strings"
	"sync"
	"time"
)

type CenterContext struct {
	Context  context.Context
	Config   *Config
	Worker   *sync.WaitGroup
	Registry *prometheus.Registry
	Echo     *echo.Echo
	MQ       *natsx.NatsHelper
	DB       *dbx.GormHelper
	RDB      *dbx.RedisHelper
}

type RoomProvider interface {
	// Init from provided context
	Init(*CenterContext) error
	// Provide a live room for watch,
	// if room already added from other provider, will set provide flag for this provide
	Provide(chan<- *ProvidedRoom)
	// Revoke a provided room,
	// will unset this provider flag, if all provider unset this room, room will stop watching
	Revoke(chan<- uint64)
}

type ProvidedRoom struct {
	ProviderName string `json:"provider_name"`
	RoomID       uint64 `json:"room_id"`
	// TODO: support custom UID/Cookie/UA/Headers etc.
}

type AgentStatus struct {
	ID           string             `json:"id"`
	Mask         uint16             `json:"mask"`
	Condition    AgentCondition     `json:"condition"`
	UpdateTime   time.Time          `json:"update_time"`
	CachedStatus *agent.AgentStatus `json:"cached_status"`
	HitStatus    map[string]uint32  `json:"hit_status"`
	mu           sync.RWMutex
}

func (s *AgentStatus) IsReady() bool {
	return s.Condition&AgentInitialization > 0 && s.Condition&AgentReady > 0
}

func (s *AgentStatus) StatusString() string {
	status := make([]string, 0, 3)
	if s.Condition&AgentInitialization > 0 {
		status = append(status, "INITIALIZED")
	}
	if s.Condition&AgentReady > 0 {
		status = append(status, "READY")
	}
	if s.Condition&AgentSync > 0 {
		status = append(status, "SYNCED")
	}
	if len(status) > 0 {
		return strings.Join(status, "|")
	}
	return "NO INITIALIZED"
}

type AgentCondition uint32

var (
	AgentInitialization = AgentCondition(1 << 0)
	AgentReady          = AgentCondition(1 << 1)
	AgentSync           = AgentCondition(1 << 2)
)
