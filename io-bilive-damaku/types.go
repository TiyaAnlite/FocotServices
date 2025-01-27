package main

import (
	"context"
	"github.com/TiyaAnlite/FocotServicesCommon/dbx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
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
