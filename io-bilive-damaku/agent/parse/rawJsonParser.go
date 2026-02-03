package parse

import (
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/tidwall/gjson"
)

func Danmu(rawData string, user *agent.UserInfoMeta, medal *agent.FansMedalMeta, damaku *agent.Damaku) {
	data := gjson.Parse(rawData)
	damaku.Meta.TimeStamp = data.Get("0.4").Uint()
	user.UID = data.Get("2.0").Uint()
	user.UserName = data.Get("2.1").String()
	if face := data.Get("0.15.user.base.face").String(); face != "" {
		user.Face = &face
	}
	medal.UID = user.UID
	medal.RoomUID = data.Get("3.12").Uint()
	medal.Name = data.Get("3.1").String()
	medal.Level = uint32(data.Get("3.0").Uint())
	medal.Light = data.Get("3.11").Bool()
	medal.GuardLevel = agent.GuardLevelType(data.Get("3.10").Uint())
	damaku.UID = user.UID
	damaku.Content = data.Get("1").String()
	damaku.Medal = medal.RoomUID
}

func Gift(rawData string, user *agent.UserInfoMeta, medal *agent.FansMedalMeta, gift *agent.Gift) {
	data := gjson.Parse(rawData)
	gift.Meta.TimeStamp = data.Get("timestamp").Uint() * 1000
	user.UID = data.Get("uid").Uint()
	user.UserName = data.Get("uname").String()
	if face := data.Get("face").String(); face != "" {
		user.Face = &face
	}
	if wealth := data.Get("wealth_level").Uint(); wealth != 0 {
		uWealth := uint32(wealth)
		user.WealthLevel = &uWealth
	}
	medal.UID = user.UID
	if data.Get("medal_info").Type != gjson.Null {
		medal.RoomUID = data.Get("medal_info.target_id").Uint()
		medal.Name = data.Get("medal_info.medal_name").String()
		medal.Level = uint32(data.Get("medal_info.medal_level").Uint())
		medal.Light = data.Get("medal_info.is_lighted").Bool()
		medal.GuardLevel = agent.GuardLevelType(data.Get("medal_info.guard_level").Uint())
	}
	gift.UID = user.UID
	gift.Count = uint32(data.Get("num").Uint())
	gift.Medal = medal.RoomUID
	gift.TID = data.Get("tid").Uint()
	if gift.Info == nil {
		gift.Info = &agent.Gift_GiftInfo{}
	}
	gift.Info.ID = uint32(data.Get("giftId").Uint())
	gift.Info.Name = data.Get("giftName").String()
	gift.Info.Price = uint32(data.Get("price").Uint())
	if data.Get("blind_gift").Type != gjson.Null {
		if gift.OriginalInfo == nil {
			gift.OriginalInfo = &agent.Gift_GiftInfo{}
		}
		gift.OriginalInfo.ID = uint32(data.Get("blind_gift.original_gift_id").Uint())
		gift.OriginalInfo.Name = data.Get("blind_gift.original_gift_name").String()
		gift.OriginalInfo.Price = uint32(data.Get("blind_gift.original_gift_price").Uint())
	} else {
		gift.OriginalInfo = gift.Info
	}
}

func Guard(rawData string, user *agent.UserInfoMeta, guard *agent.Guard) {
	data := gjson.Parse(rawData)
	guard.Meta.TimeStamp = data.Get("start_time").Uint() * 1000
	user.UID = data.Get("uid").Uint()
	user.UserName = data.Get("username").String()
	guard.UID = user.UID
	guard.Price = uint32(data.Get("price").Uint())
	guard.GiftType = agent.Guard_GuardGiftType(data.Get("gift_id").Uint())
}

func SuperChat(rawData string, user *agent.UserInfoMeta, medal *agent.FansMedalMeta, sc *agent.SuperChat) {
	data := gjson.Parse(rawData)
	sc.Meta.TimeStamp = data.Get("ts").Uint()
	user.UID = data.Get("uid").Uint()
	user.UserName = data.Get("uinfo.base.name").String()
	face := data.Get("uinfo.base.face").String()
	user.Face = &face
	uLevel := uint32(data.Get("user_info.user_level").Uint())
	user.Level = &uLevel
	medal.UID = user.UID
	if data.Get("uinfo.medal").Type != gjson.Null {
		medal.RoomUID = data.Get("uinfo.medal.ruid").Uint()
		medal.Name = data.Get("uinfo.medal.name").String()
		medal.Level = uint32(data.Get("uinfo.medal.level").Uint())
		medal.Light = data.Get("uinfo.medal.is_light").Bool()
		medal.GuardLevel = agent.GuardLevelType(data.Get("uinfo.medal.guard_level").Uint())
	}
	sc.ID = data.Get("id").Uint()
	sc.UID = user.UID
	sc.Message = data.Get("message").String()
	sc.MessageTrans = data.Get("message_trans").String()
	sc.Price = uint32(data.Get("price").Uint())
	sc.Medal = medal.RoomUID
}
