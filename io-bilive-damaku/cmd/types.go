package main

import (
	"time"

	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
)

type BliveData struct {
	Meta struct {
		RecorderVersion string    `json:"RecorderVersion" xml:"xattr:version"`
		RoomID          int64     `json:"RoomID"`
		ShortRoomID     int64     `json:"ShortRoomID"`
		Name            string    `json:"Name"`
		Title           string    `json:"Title"`
		AreaNameParent  string    `json:"AreaNameParent"`
		AreaNameChild   string    `json:"AreaNameChild"`
		StartTime       time.Time `json:"StartTime"`
	} `json:"Meta"`
	Damaku    []*agent.Damaku        `json:"Damaku"`
	Gift      []*agent.Gift          `json:"Gift"`
	Guard     []*agent.Guard         `json:"Guard"`
	SuperChat []*agent.SuperChat     `json:"SuperChat"`
	User      []*agent.UserInfoMeta  `json:"User"`
	FansMedal []*agent.FansMedalMeta `json:"FansMedal"`
	mapUser   map[uint64]*agent.UserInfoMeta
	mapMedal  map[uint64]map[uint64]*agent.FansMedalMeta // UID:RoomID:Medal
}

type BliveDanmaku struct {
	agent.Damaku
	TimeStamp uint64 `json:"TimeStamp"`
	Meta      any    `json:"-"`
}

type BliveGift struct {
	agent.Gift
	TimeStamp uint64 `json:"TimeStamp"`
	Meta      any    `json:"-"`
}

type BliveGuard struct {
}

type BliveSuperChat struct {
}
