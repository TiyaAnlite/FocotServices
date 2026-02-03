package main

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	biliChat "github.com/FishZe/go-bili-chat/v2"
	biliChatClient "github.com/FishZe/go-bili-chat/v2/client"
	"github.com/FishZe/go-bili-chat/v2/events"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/allegro/bigcache/v3"
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
	"github.com/duke-git/lancet/v2/compare"
	"github.com/duke-git/lancet/v2/condition"
	"github.com/nats-io/nats.go"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"k8s.io/klog/v2"
)

var (
	SupportedMsgTypes = []string{
		events.CmdDanmuMsg,
		events.CmdSendGift,
		events.CmdGuardBuy,
		events.CmdSuperChatMessage,
		events.CmdOnlineRankCount,
		events.CmdOnlineRankV2,
	}
	SupportedMsgTypesNames = map[string]string{
		events.CmdDanmuMsg:         "Damaku",
		events.CmdSendGift:         "Gift",
		events.CmdGuardBuy:         "Guard",
		events.CmdSuperChatMessage: "SuperChat",
		events.CmdOnlineRankCount:  "OnlineRank",
		events.CmdOnlineRankV2:     "OnlineRankV2",
	}
	SupportedMsgTypesProto = map[string]agent.AgentStatus_BufferType{
		events.CmdDanmuMsg:         agent.AgentStatus_Damaku,
		events.CmdSendGift:         agent.AgentStatus_Gift,
		events.CmdGuardBuy:         agent.AgentStatus_Guard,
		events.CmdSuperChatMessage: agent.AgentStatus_SuperChat,
		events.CmdOnlineRankCount:  agent.AgentStatus_OnlineRank,
		events.CmdOnlineRankV2:     agent.AgentStatus_OnlineRankV2,
	}
	CacheConfig = bigcache.Config{
		Shards:           1024,
		LifeWindow:       time.Minute * 30,
		CleanWindow:      time.Hour,
		MaxEntrySize:     500,
		HardMaxCacheSize: 0,
	}
)

type DamakuCenterAgent struct {
	chatHandler   *biliChat.Handler
	controlChan   chan *nats.Msg
	eventChan     chan *BLiveEventHandlerMsg
	eventCounter  map[string]*atomic.Int32
	watchingRooms sync.Map
	metaBuilder   agent.MetaBuilder

	// runtime channel
	userMetaChan  chan *agent.UserInfoMeta
	medalMetaChan chan *agent.FansMedalMeta

	// runtime cache
	userMetaCache  *bigcache.BigCache
	medalMetaCache *bigcache.BigCache
}

func (a *DamakuCenterAgent) Init() {
	var err error
	a.chatHandler = biliChat.GetNewHandler()
	a.controlChan = make(chan *nats.Msg, 4)
	a.eventChan = make(chan *BLiveEventHandlerMsg, 100)
	a.eventCounter = make(map[string]*atomic.Int32)
	a.metaBuilder = agent.NewMsgMetaBuilder(cfg.AgentId)
	a.userMetaChan = make(chan *agent.UserInfoMeta, 32)
	a.medalMetaChan = make(chan *agent.FansMedalMeta, 32)
	a.userMetaCache, err = bigcache.New(ctx, CacheConfig)
	if err != nil {
		klog.Fatalf("init cache failed: %s", err.Error())
	}
	a.medalMetaCache, err = bigcache.New(ctx, CacheConfig)
	if err != nil {
		klog.Fatalf("init cache failed: %s", err.Error())
	}
	registerPacket := &agent.AgentInfo{ID: cfg.AgentId, Type: agent.AgentInfo_RealTimeAgent}
	registerData, err := proto.Marshal(registerPacket)
	if err != nil {
		klog.Fatalf("marshal register packet failed: %s", err.Error())
	}
	klog.Info("agent init started")
	registerChan := make(chan *nats.Msg, 1)
	registerSub, err := mq.Nc.ChanSubscribe(fmt.Sprintf("%s.agent.%s.init", cfg.SubjectPrefix, cfg.AgentId), registerChan)
	if err != nil {
		klog.Fatalf("subscribe init subject failed: %s", err.Error())
	}
	ticker := time.NewTicker(time.Second * 3)
initLoop:
	for {
		select {
		case <-ticker.C:
			if err := mq.Publish(fmt.Sprintf("%s.agent.info", cfg.SubjectPrefix), registerData); err != nil {
				klog.Errorf("publish register msg failed: %s", err.Error())
			}
		case msg := <-registerChan:
			regMsg := &agent.AgentInit{}
			if err := proto.Unmarshal(msg.Data, regMsg); err != nil {
				klog.Errorf("unmarshal register msg failed: %s", err.Error())
				continue
			}
			// setup bili client
			biliChat.SetHeaderCookie(regMsg.Cookie)
			biliChat.SetBuvid(regMsg.BUVID)
			biliChat.SetUID(int64(regMsg.UID))
			if regMsg.UA != nil {
				biliChat.SetHeaderUA(regMsg.GetUA())
			}
			if regMsg.PriorityMode != nil {
				biliChat.SetClientPriorityMode(int(regMsg.GetPriorityMode()))
			}
			for k, v := range regMsg.GetHeader() {
				biliChatClient.Header.Set(k, v)
			}
			controlSub, err := mq.Nc.ChanSubscribe(fmt.Sprintf("%s.agent.%s.action", cfg.SubjectPrefix, cfg.AgentId), a.controlChan)
			if err != nil {
				klog.Errorf("subscribe control subject failed: %s", err.Error())
				_ = agent.ControlError(msg, err)
				continue
			}
			mq.AddSubscribe(controlSub)
			if err := agent.ControlSuccess(msg); err != nil {
				klog.Errorf("response control msg failed: %s", err.Error()) // is error will be ignored
			}
			ticker.Stop()
			_ = registerSub.Unsubscribe()
			break initLoop
		}
	}
	// start
	klog.Infof("agent initialized, UID: %d", biliChatClient.UID)
	go a.chatHandler.Run()
	// all supported handler here
	for _, msgType := range SupportedMsgTypes {
		a.eventCounter[msgType] = &atomic.Int32{}
		a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: msgType, EventChan: a.eventChan, Counter: a.eventCounter[msgType]})
	}
	go a.controller()
	go a.eventHandler()
	go a.metaIndexer()
}

// collect status and receive control action
func (a *DamakuCenterAgent) controller() {
	collectTicker := time.NewTicker(time.Second)
	worker.Add(1)
	klog.Info("controller started")
	for {
		select {
		case <-collectTicker.C:
			status := &agent.AgentStatus{
				Meta:             a.metaBuilder(),
				BufferUsed:       uint32(len(a.eventChan)),
				BufferEventCount: make(map[int32]int32),
				MetaCache:        make(map[int32]*agent.AgentStatus_MetaCacheInfo),
			}
			a.userMetaCache.Len()
			a.watchingRooms.Range(func(key, _ any) bool {
				uKey, ok := key.(uint64)
				if !ok {
					klog.Errorf("key: %+v not a uint64", key)
					return true
				}
				status.Watching = append(status.Watching, uKey)
				return true
			})
			for k, counter := range a.eventCounter {
				status.BufferEventCount[int32(SupportedMsgTypesProto[k])] = counter.Load()
			}
			userCacheStatus := a.userMetaCache.Stats()
			medalCacheStatus := a.medalMetaCache.Stats()
			status.MetaCache[int32(agent.AgentStatus_User)] = &agent.AgentStatus_MetaCacheInfo{
				Buffer:     uint32(len(a.userMetaChan)),
				Cached:     uint32(a.userMetaCache.Len()),
				Hits:       userCacheStatus.Hits,
				Misses:     userCacheStatus.Misses,
				DelHits:    userCacheStatus.DelHits,
				DelMisses:  userCacheStatus.DelMisses,
				Collisions: userCacheStatus.Collisions,
			}
			status.MetaCache[int32(agent.AgentStatus_Medal)] = &agent.AgentStatus_MetaCacheInfo{
				Buffer:     uint32(len(a.medalMetaChan)),
				Cached:     uint32(a.medalMetaCache.Len()),
				Hits:       medalCacheStatus.Hits,
				Misses:     medalCacheStatus.Misses,
				DelHits:    medalCacheStatus.DelHits,
				DelMisses:  medalCacheStatus.DelMisses,
				Collisions: medalCacheStatus.Collisions,
			}
			statusData, err := proto.Marshal(status)
			if err != nil {
				klog.Errorf("failed to marshal status data: %s", err.Error())
			}
			if err := mq.Publish(fmt.Sprintf("%s.agent.status", cfg.SubjectPrefix), statusData); err != nil {
				klog.Errorf("publish status msg failed: %s", err.Error())
			}
		case controlMsg := <-a.controlChan:
			action := &agent.AgentAction{}
			if err := proto.Unmarshal(controlMsg.Data, action); err != nil {
				klog.Errorf("unmarshal agent action failed: %s", err.Error())
			}
			switch action.Type {
			case agent.AgentAction_AddRoom, agent.AgentAction_DelRoom:
				if action.RoomID == nil {
					klog.Errorf("no room id")
					if err := agent.ControlError(controlMsg, errors.New("no room id")); err != nil {
						klog.Errorf("response control msg failed: %s", err.Error())
					}
				}
				//if *action.RoomID < 10000 {
				//	klog.Errorf("unsupported room id: %d", *action.RoomID)
				//	_ = agent.ControlError(controlMsg, fmt.Errorf("unsupported room id: %d", *action.RoomID))
				//}
				klog.V(3).Infof("action: %s %d", agent.AgentAction_AgentActionType_name[int32(action.Type)], action.RoomID)
				if action.Type == agent.AgentAction_AddRoom {
					_ = a.chatHandler.AddRoom(int(*action.RoomID))
					a.watchingRooms.Store(*action.RoomID, struct{}{})
				} else if action.Type == agent.AgentAction_DelRoom {
					_ = a.chatHandler.DelRoom(int(*action.RoomID))
					a.watchingRooms.Delete(*action.RoomID)
				}
				if err := agent.ControlSuccess(controlMsg); err != nil {
					klog.Errorf("response control msg failed: %s", err.Error())
				}
			}
		case <-ctx.Done():
			klog.Infof("agent controller stopped")
			worker.Done()
			return
		}
	}
}

// can be parallelization
func (a *DamakuCenterAgent) eventHandler() {
	worker.Add(1)
	klog.Info("event handler started")
	for {
		select {
		case msg := <-a.eventChan:
			a.eventCounter[msg.event.Cmd].Add(-1) // counter
			msg.processTime = time.Now()
			roomId := uint64(msg.event.RoomId)
			switch msg.event.Cmd {
			case events.CmdDanmuMsg:
				// init
				userMeta := &agent.UserInfoMeta{}
				meta := a.metaBuilder()
				meta.RoomID = &roomId
				// parse
				data := gjson.ParseBytes(msg.event.RawMessage)
				meta.TimeStamp = data.Get("info.0.4").Uint()
				userMeta.UID = data.Get("info.2.0").Uint()
				userMeta.UserName = data.Get("info.2.1").String()
				if userFace := data.Get("info.0.15.user.base.face").String(); userFace != "" {
					userMeta.Face = &userFace
				}
				a.userMetaChan <- userMeta

				medal := &agent.FansMedalMeta{
					UID:        userMeta.UID,
					RoomUID:    data.Get("info.3.12").Uint(),
					Name:       data.Get("info.3.1").String(),
					Level:      uint32(data.Get("info.3.0").Uint()),
					Light:      data.Get("info.3.11").Bool(),
					GuardLevel: agent.GuardLevelType(data.Get("info.3.10").Uint()),
				}
				a.medalMetaChan <- medal

				danmaku := &agent.Damaku{
					Meta:    meta,
					UID:     userMeta.UID,
					Content: data.Get("info.1").String(),
					Medal:   medal.RoomUID,
				}
				// trace
				danmaku.Meta.Trace[int32(agent.BasicMsgMeta_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Microseconds())
				danmaku.Meta.Trace[int32(agent.BasicMsgMeta_Process)] = uint64(time.Now().Sub(msg.processTime).Microseconds())
				sendData, err := proto.Marshal(danmaku)
				if err != nil {
					klog.Errorf("failed to marshal danmaku: %s", err.Error())
					continue
				}
				if err := mq.Publish(fmt.Sprintf("%s.stream.damaku", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish danmaku message failed: %s", err.Error())
				}
				klog.V(5).Infof("danmaku push")
			case events.CmdSendGift:
				var giftData events.SendGift
				var extraGiftData ExtraSendGiftEvent
				if err := sonic.Unmarshal(msg.event.RawMessage, &giftData); err != nil {
					klog.Errorf("failed to unmarshal gift data: %s", err.Error())
					continue
				}
				if err := sonic.Unmarshal(msg.event.RawMessage, &extraGiftData); err != nil {
					klog.Errorf("failed to unmarshal extra gift data: %s", err.Error())
					continue
				}
				userMeta := &agent.UserInfoMeta{
					UID:      uint64(giftData.Data.UID),
					UserName: giftData.Data.Name,
				}
				if giftData.Data.Face != "" {
					userMeta.Face = &giftData.Data.Face
				}
				if extraGiftData.Data.WealthLevel != 0 {
					userMeta.WealthLevel = &extraGiftData.Data.WealthLevel
				}
				a.userMetaChan <- userMeta

				medal := &agent.FansMedalMeta{
					UID: userMeta.UID,
				}
				if giftData.Data.FansMedal != nil {
					medal.RoomUID = uint64(giftData.Data.FansMedal.TargetId)
					medal.Name = giftData.Data.FansMedal.MedalName
					medal.Level = uint32(giftData.Data.FansMedal.MedalLevel)
					medal.Light = condition.TernaryOperator(giftData.Data.FansMedal.IsLighted, true, false)
					medal.GuardLevel = agent.GuardLevelType(giftData.Data.FansMedal.GuardLevel)
				}
				a.medalMetaChan <- medal

				gift := &agent.Gift{
					Meta:  a.metaBuilder(),
					UID:   userMeta.UID,
					Count: uint32(giftData.Data.Num),
					Medal: medal.RoomUID,
				}
				gift.Meta.RoomID = &roomId
				gift.Meta.TimeStamp = uint64(giftData.Data.Timestamp * 1000)
				giftId, err := strconv.ParseInt(giftData.Data.Tid, 10, 64)
				if err != nil {
					klog.Errorf("failed to parse gift id(%s): %s", giftData.Data.Rnd, err.Error())
					continue
				}
				gift.TID = uint64(giftId)
				gift.Info = &agent.Gift_GiftInfo{
					ID:    uint32(giftData.Data.GiftID),
					Name:  giftData.Data.GiftName,
					Price: uint32(giftData.Data.Price),
				}
				if giftData.Data.BlindGift != nil {
					gift.OriginalInfo = &agent.Gift_GiftInfo{
						ID:    uint32(giftData.Data.BlindGift.OriginalGiftId),
						Name:  giftData.Data.BlindGift.OriginalGiftName,
						Price: uint32(giftData.Data.BlindGift.OriginalGiftPrice),
					}
				} else {
					gift.OriginalInfo = gift.Info
				}
				// trace
				gift.Meta.Trace[int32(agent.BasicMsgMeta_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Microseconds())
				gift.Meta.Trace[int32(agent.BasicMsgMeta_Process)] = uint64(time.Now().Sub(msg.processTime).Microseconds())
				sendData, err := proto.Marshal(gift)
				if err != nil {
					klog.Errorf("failed to marshal gift: %s", err.Error())
					continue
				}
				if err := mq.Publish(fmt.Sprintf("%s.stream.gift", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish gift message failed: %s", err.Error())
				}
				klog.V(5).Infof("gift push")
			case events.CmdGuardBuy:
				var guardData events.GuardBuyMsg
				if err := sonic.Unmarshal(msg.event.RawMessage, &guardData); err != nil {
					klog.Errorf("failed to unmarshal gift data: %s", err.Error())
					continue
				}
				userMeta := &agent.UserInfoMeta{
					UID:      uint64(guardData.Data.UID),
					UserName: guardData.Data.Username,
				}
				a.userMetaChan <- userMeta

				guard := &agent.Guard{
					Meta:     a.metaBuilder(),
					UID:      userMeta.UID,
					Price:    uint32(guardData.Data.Price),
					GiftType: agent.Guard_GuardGiftType(guardData.Data.GiftID),
				}
				guard.Meta.RoomID = &roomId
				guard.Meta.TimeStamp = uint64(guardData.Data.StartTime * 1000)
				// trace
				guard.Meta.Trace[int32(agent.BasicMsgMeta_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Microseconds())
				guard.Meta.Trace[int32(agent.BasicMsgMeta_Process)] = uint64(time.Now().Sub(msg.processTime).Microseconds())
				sendData, err := proto.Marshal(guard)
				if err != nil {
					klog.Errorf("failed to marshal guard: %s", err.Error())
					continue
				}
				if err := mq.Publish(fmt.Sprintf("%s.stream.guard", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish guard message failed: %s", err.Error())
				}
				klog.V(5).Infof("guard push")
			case events.CmdSuperChatMessage:
				var scData events.SuperChatMessage
				if err := sonic.Unmarshal(msg.event.RawMessage, &scData); err != nil {
					// TODO: sc data bug: medal_info.medal_color is not a int64
					var e *decoder.MismatchTypeError
					if !errors.As(err, &e) {
						klog.Errorf("failed to unmarshal superchat data: %s", err.Error())
						continue
					}
				}
				userMeta := &agent.UserInfoMeta{
					UID:      uint64(scData.Data.Uid),
					UserName: scData.Data.UInfo.Base.Name,
					Face:     &scData.Data.UInfo.Base.Face,
				}
				uLevel := uint32(scData.Data.UserInfo.UserLevel)
				userMeta.Level = &uLevel
				a.userMetaChan <- userMeta

				medal := &agent.FansMedalMeta{
					UID: userMeta.UID,
				}
				if scData.Data.UInfo.Medal.Ruid != 0 {
					medal.RoomUID = uint64(scData.Data.UInfo.Medal.Ruid)
					medal.Name = scData.Data.UInfo.Medal.Name
					medal.Level = uint32(scData.Data.UInfo.Medal.Level)
					medal.Light = condition.TernaryOperator(scData.Data.UInfo.Medal.IsLight, true, false)
					medal.GuardLevel = agent.GuardLevelType(uint32(scData.Data.UInfo.Medal.GuardLevel))
				}
				a.medalMetaChan <- medal

				sc := &agent.SuperChat{
					Meta:         a.metaBuilder(),
					ID:           uint64(scData.Data.Id),
					UID:          userMeta.UID,
					Message:      scData.Data.Message,
					MessageTrans: scData.Data.MessageTrans,
					Price:        uint32(scData.Data.Price),
					Medal:        medal.RoomUID,
				}
				sc.Meta.RoomID = &roomId
				sc.Meta.TimeStamp = uint64(scData.Data.Ts)
				// trace
				sc.Meta.Trace[int32(agent.BasicMsgMeta_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Microseconds())
				sc.Meta.Trace[int32(agent.BasicMsgMeta_Process)] = uint64(time.Now().Sub(msg.processTime).Microseconds())
				sendData, err := proto.Marshal(sc)
				if err != nil {
					klog.Errorf("failed to marshal superChat: %s", err.Error())
					continue
				}
				if err := mq.Publish(fmt.Sprintf("%s.stream.superChat", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish superChat message failed: %s", err.Error())
				}
				klog.V(5).Infof("superChat push")
			case events.CmdOnlineRankCount:
				meta := a.metaBuilder()
				var orData OnlineRankCount
				if err := sonic.Unmarshal(msg.event.RawMessage, &orData); err != nil {
					klog.Errorf("failed to unmarshal online rank count: %s", err.Error())
					continue
				}
				or := &agent.OnlineRankCount{
					Meta:   meta,
					Count:  orData.Data.Count,
					Online: orData.Data.Online,
				}
				or.Meta.RoomID = &roomId
				// trace
				or.Meta.Trace[int32(agent.BasicMsgMeta_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Microseconds())
				or.Meta.Trace[int32(agent.BasicMsgMeta_Process)] = uint64(time.Now().Sub(msg.processTime).Microseconds())
				sendData, err := proto.Marshal(or)
				if err != nil {
					klog.Errorf("failed to marshal onlineRankCount: %s", err.Error())
					continue
				}
				if err := mq.Publish(fmt.Sprintf("%s.stream.online", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish onlineRankCount message failed: %s", err.Error())
				}
				klog.V(5).Infof("onlineRankCount push")
			case events.CmdOnlineRankV2:
				meta := a.metaBuilder()
				var or2Data OnlineRankV2
				if err := sonic.Unmarshal(msg.event.RawMessage, &or2Data); err != nil {
					klog.Errorf("failed to unmarshal onlineRankV2: %s", err.Error())
					continue
				}
				var rankList []*agent.OnlineRankV2_OnlineRankList
				for _, rank := range or2Data.Data.OnlineList {
					userMeta := &agent.UserInfoMeta{
						UID:      uint64(rank.UID),
						UserName: rank.Name,
						Face:     &rank.Face,
					}
					a.userMetaChan <- userMeta

					rankScore, err := strconv.ParseUint(rank.Score, 10, 32)
					if err != nil {
						klog.Errorf("failed to parse rank score: %s", err.Error())
						continue
					}
					rankList = append(rankList, &agent.OnlineRankV2_OnlineRankList{
						Rank:       uint32(rank.Rank),
						Score:      uint32(rankScore),
						UID:        userMeta.UID,
						GuardLevel: agent.GuardLevelType(rank.GuardLevel),
					})
				}
				or2 := &agent.OnlineRankV2{
					Meta: meta,
					List: rankList,
				}
				or2.Meta.RoomID = &roomId
				// trace
				or2.Meta.Trace[int32(agent.BasicMsgMeta_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Microseconds())
				or2.Meta.Trace[int32(agent.BasicMsgMeta_Process)] = uint64(time.Now().Sub(msg.processTime).Microseconds())
				sendData, err := proto.Marshal(or2)
				if err != nil {
					klog.Errorf("failed to marshal onlineRankV2: %s", err.Error())
					continue
				}
				if err := mq.Publish(fmt.Sprintf("%s.stream.onlineV2", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish onlineRankV2 message failed: %s", err.Error())
				}
				klog.V(5).Infof("onlineRankV2 push")
			default:
				klog.Warningf("unsupported command: %s", msg.event.Cmd)
				continue
			}
		case <-ctx.Done():
			klog.Infof("agent event handler stopped")
			worker.Done()
			return
		}
	}
}

// update & sync user/medal meta cache
func (a *DamakuCenterAgent) metaIndexer() {
	worker.Add(1)
	klog.Info("meta indexer started")
	syncMeta := func(cache *bigcache.BigCache, cacheKey, syncSubject string, meta protoreflect.ProtoMessage) {
		// update cache
		data, err := proto.Marshal(meta)
		if err != nil {
			klog.Errorf("failed to marshal meta data: %s", err.Error())
			return
		}
		klog.V(5).Infof("meta(%s) push: %s", syncSubject, cacheKey)
		if err := mq.Publish(fmt.Sprintf("%s.stream.%s", cfg.SubjectPrefix, syncSubject), data); err != nil {
			klog.Errorf("publish %s meta message failed: %s", syncSubject, err.Error())
		}
		if err := cache.Set(cacheKey, data); err != nil {
			klog.Errorf("failed to cache meta: %s", err.Error())
			return
		}
	}
	for {
		select {
		case meta := <-a.userMetaChan:
			// checking cache
			userKey := strconv.FormatUint(meta.UID, 10)
			cached, err := a.userMetaCache.Get(userKey)
			if err != nil {
				if errors.Is(err, bigcache.ErrEntryNotFound) {
					// no cache sync
					syncMeta(a.userMetaCache, userKey, "userInfo", meta)
					continue
				}
				klog.Errorf("failed to get cached user meta: %s", err.Error())
				continue
			}
			var cachedMeta agent.UserInfoMeta
			if err := proto.Unmarshal(cached, &cachedMeta); err != nil {
				klog.Errorf("failed to unmarshal cached user meta: %s", err.Error())
				continue
			}
			if meta.UserName == cachedMeta.UserName &&
				compare.Equal(meta.Face, cachedMeta.Face) &&
				compare.Equal(meta.Level, cachedMeta.Level) &&
				compare.Equal(meta.WealthLevel, cachedMeta.WealthLevel) {
				continue
			}
			// diff compare
			if meta.Face == nil && cachedMeta.Face != nil {
				meta.Face = cachedMeta.Face
			}
			if meta.Level == nil && cachedMeta.Level != nil {
				meta.Level = cachedMeta.Level
			}
			if meta.WealthLevel == nil && cachedMeta.WealthLevel != nil {
				meta.WealthLevel = cachedMeta.WealthLevel
			}
			// update sync
			syncMeta(a.userMetaCache, userKey, "userInfoMeta", meta)
		case meta := <-a.medalMetaChan:
			if meta.RoomUID == 0 {
				// not medal, skip
				continue
			}
			medalKey := fmt.Sprintf("%d:%d", meta.UID, meta.RoomUID)
			cached, err := a.medalMetaCache.Get(medalKey)
			if err != nil {
				if errors.Is(err, bigcache.ErrEntryNotFound) {
					syncMeta(a.medalMetaCache, medalKey, "fansMedal", meta)
					continue
				}
				klog.Errorf("failed to get cached medal meta: %s", err.Error())
				continue
			}
			var cachedMeta agent.FansMedalMeta
			if err := proto.Unmarshal(cached, &cachedMeta); err != nil {
				klog.Errorf("failed to unmarshal cached medal meta: %s", err.Error())
				continue
			}
			// FansMedalMeta is full update, not need to compare diff
			if meta.RoomUID == cachedMeta.RoomUID &&
				meta.Name == cachedMeta.Name &&
				meta.Level == cachedMeta.Level &&
				meta.Light == cachedMeta.Light &&
				meta.GuardLevel == cachedMeta.GuardLevel {
				continue
			}
			syncMeta(a.medalMetaCache, medalKey, "fansMedal", meta)
		case <-ctx.Done():
			klog.Infof("meta indexer stopped")
			worker.Done()
			return
		}
	}
}
