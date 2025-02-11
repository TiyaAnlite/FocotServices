package main

import (
	"errors"
	"fmt"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/allegro/bigcache/v3"
	"github.com/duke-git/lancet/v2/compare"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
	"time"
)

type DamakuController struct {
	agent          *AgentManager
	storage        *StorageController
	metrics        *MetricsService
	centerCtx      *CenterContext
	providers      []RoomProvider
	streamChan     chan *nats.Msg
	eventChan      chan any
	dupCache       *bigcache.BigCache // [msgType]:[msgUniqueKey]
	userMetaCache  *bigcache.BigCache // UserInfoMeta: uid
	medalMetaCache *bigcache.BigCache // FansMedalMeta: user:rid
}

func (c *DamakuController) Init(ctx *CenterContext, providers []RoomProvider) error {
	var err error
	c.agent = &AgentManager{}
	c.storage = &StorageController{}
	c.metrics = &MetricsService{}
	c.centerCtx = ctx
	c.providers = providers
	c.streamChan = make(chan *nats.Msg, 100)
	c.eventChan = make(chan any, 200)
	c.dupCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:      1024,
		LifeWindow:  ctx.Config.Controller.DuplicateWindow,
		CleanWindow: time.Minute,
		OnRemove:    c.dupCacheRelease,
		Logger:      klog.NewStandardLogger("INFO"),
	})
	if err != nil {
		return fmt.Errorf("failed to init dupCache: %s", err.Error())
	}
	c.userMetaCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:           1024,
		LifeWindow:       time.Minute * 30,
		CleanWindow:      time.Hour,
		MaxEntrySize:     500,
		HardMaxCacheSize: 0,
		Logger:           klog.NewStandardLogger("INFO"),
	})
	if err != nil {
		return fmt.Errorf("failed to init userMetaCache: %s", err.Error())
	}
	c.medalMetaCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:           1024,
		LifeWindow:       time.Minute * 30,
		CleanWindow:      time.Hour,
		MaxEntrySize:     500,
		HardMaxCacheSize: 0,
		Logger:           klog.NewStandardLogger("INFO"),
	})
	if err != nil {
		return fmt.Errorf("failed to init medalMetaCache: %s", err.Error())
	}
	if err := c.agent.Init(c.centerCtx); err != nil {
		return fmt.Errorf("failed to init agent manager: %s", err.Error())
	}
	chanProvide, chanRevoke := c.agent.GetRoomChan()
	for _, provider := range providers {
		provider.Provide(chanProvide)
		provider.Revoke(chanRevoke)
	}
	sub, err := c.centerCtx.MQ.Nc.ChanSubscribe(fmt.Sprintf("%s.stream.*", c.centerCtx.Config.Global.Prefix), c.streamChan)
	if err != nil {
		return fmt.Errorf("failed to subscribe stream msg: %s", err.Error())
	}
	c.centerCtx.MQ.AddSubscribe(sub)
	return nil
}

// receive agent message and process duplicate, msg subject should be *.agent.*, can be parallelization
func (c *DamakuController) aggregateWindow() {
	c.centerCtx.Worker.Add(1)
	defer c.centerCtx.Worker.Done()
	msgDupFilter := func(msgType, key string, msg proto.Message, mask []byte) error {
		if _, err := c.dupCache.Get(key); err == nil {
			if errors.Is(err, bigcache.ErrEntryNotFound) {
				// cache miss, add it
				c.eventChan <- msg
			} else {
				klog.Errorf("failed to get cached %s: %s", msgType, err.Error())
				return err
			}
		}
		// add flag
		if err := c.dupCache.Append(key, mask); err != nil {
			klog.Errorf("failed to append cache for damaku: %s", err.Error())
			return err
		}
		return nil
	}
	klog.Info("aggregate window start")
	for {
		select {
		case msg := <-c.streamChan:
			subject := strings.Split(msg.Subject, ".")
			switch subject[len(subject)-1] {
			// meta msg will unmarshal first, then compare diff from cache
			case "fansMedal":
				meta := &agent.FansMedalMeta{}
				if err := proto.Unmarshal(msg.Data, meta); err != nil {
					klog.Errorf("failed to unmarshal agent fans medal: %s", err.Error())
					_ = agent.ControlError(msg, err)
					continue
				}
				if meta.RoomUID == 0 {
					klog.Warning("agent fans medal room uid is zero")
					continue
				}
				medalKey := fmt.Sprintf("%d:%d", meta.UID, meta.RoomUID)
				cached, err := c.medalMetaCache.Get(medalKey)
				if err != nil {
					if errors.Is(err, bigcache.ErrEntryNotFound) {
						c.eventChan <- meta
						continue
					}
					klog.Errorf("failed to get cached medal meta: %s", err.Error())
					_ = agent.ControlError(msg, err)
					continue
				}
				var cachedMeta agent.FansMedalMeta
				if err := proto.Unmarshal(cached, &cachedMeta); err != nil {
					klog.Errorf("failed to unmarshal cached medal meta: %s", err.Error())
					_ = agent.ControlError(msg, err)
					continue
				}
				_ = agent.ControlSuccess(msg)
				// same as agent/agent.go:633
				if meta.RoomUID == cachedMeta.RoomUID &&
					meta.Name == cachedMeta.Name &&
					meta.Level == cachedMeta.Level &&
					meta.Light == cachedMeta.Light &&
					meta.GuardLevel == cachedMeta.GuardLevel {
					continue
				}
				c.eventChan <- meta
			case "userInfoMeta":
				meta := &agent.UserInfoMeta{}
				if err := proto.Unmarshal(msg.Data, meta); err != nil {
					klog.Errorf("failed to unmarshal agent fans medal: %s", err.Error())
					_ = agent.ControlError(msg, err)
					continue
				}
				userKey := strconv.FormatUint(meta.UID, 10)
				cached, err := c.userMetaCache.Get(userKey)
				if err != nil {
					if errors.Is(err, bigcache.ErrEntryNotFound) {
						c.eventChan <- meta
						continue
					}
					klog.Errorf("failed to get cached user meta: %s", err.Error())
					continue
				}
				var cachedMeta agent.UserInfoMeta
				if err := proto.Unmarshal(cached, &cachedMeta); err != nil {
					klog.Errorf("failed to unmarshal cached user meta: %s", err.Error())
					_ = agent.ControlError(msg, err)
					continue
				}
				_ = agent.ControlSuccess(msg)
				// same as agent/agent.go:595
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
				c.eventChan <- meta
			// standard msg will unmarshal first, then aggregate the message
			case "damaku":
				damaku := &agent.Damaku{}
				if err := proto.Unmarshal(msg.Data, damaku); err != nil {
					klog.Errorf("failed to unmarshal damaku: %s", err.Error())
					continue
				}
				mask := c.agent.AgentMask(damaku.Meta.Agent)
				if mask == nil {
					continue
				}
				if damaku.Meta.RoomID == nil {
					klog.Warningf("damaku meta room uid is zero")
					continue
				}
				// unique key: damaku roomID:uid:timestamp
				key := fmt.Sprintf("%s:%d:%d:%d", "damaku", *damaku.Meta.RoomID, damaku.UID, damaku.Meta.TimeStamp)
				if err := msgDupFilter("damaku", key, damaku, mask); err != nil {
					continue
				}
			case "gift":
				gift := &agent.Gift{}
				if err := proto.Unmarshal(msg.Data, gift); err != nil {
					klog.Errorf("failed to unmarshal gift: %s", err.Error())
					continue
				}
				mask := c.agent.AgentMask(gift.Meta.Agent)
				if mask == nil {
					continue
				}
				// unique key: TID
				key := fmt.Sprintf("%s:%d", "gift", gift.TID)
				if err := msgDupFilter("gift", key, gift, mask); err != nil {
					continue
				}
			case "guard":
				guard := &agent.Guard{}
				if err := proto.Unmarshal(msg.Data, guard); err != nil {
					klog.Errorf("failed to unmarshal guard: %s", err.Error())
					continue
				}
				mask := c.agent.AgentMask(guard.Meta.Agent)
				if mask == nil {
					continue
				}
				// unique key: UID:timestamp
				key := fmt.Sprintf("%s:%d:%d", "guard", guard.UID, guard.Meta.TimeStamp)
				if err := msgDupFilter("guard", key, guard, mask); err != nil {
					continue
				}
			case "superChat":
				sc := &agent.SuperChat{}
				if err := proto.Unmarshal(msg.Data, sc); err != nil {
					klog.Errorf("failed to unmarshal superChat: %s", err.Error())
					continue
				}
				mask := c.agent.AgentMask(sc.Meta.Agent)
				if mask == nil {
					continue
				}
				// unique key: UID:timestamp
				key := fmt.Sprintf("%s:%d:%d", "superChat", sc.UID, sc.Meta.TimeStamp)
				if err := msgDupFilter("superChat", key, sc, mask); err != nil {
					continue
				}
			// single stream only follow one agent at time, no duplicate check
			case "online":
				o := &agent.OnlineRankCount{}
				if err := proto.Unmarshal(msg.Data, o); err != nil {
					klog.Errorf("failed to unmarshal online: %s", err.Error())
					continue
				}
				if o.Meta.Agent != c.agent.MasterAgent() {
					continue
				}
				c.eventChan <- o
			case "onlineV2":
				o := &agent.OnlineRankV2{}
				if err := proto.Unmarshal(msg.Data, o); err != nil {
					klog.Errorf("failed to unmarshal online: %s", err.Error())
					continue
				}
				if o.Meta.Agent != c.agent.MasterAgent() {
					continue
				}
				c.eventChan <- o
			}
		case <-c.centerCtx.Context.Done():
			klog.Infof("aggregate window stpooed")
			return
		}
	}
}

// pop cached msg info and push msg hit metrics
func (c *DamakuController) dupCacheRelease(key string, entry []byte) {
	m := strings.Split(key, ":")
	if len(m) < 2 {
		return
	}
	c.agent.AddAgentHitMask(entry, m[0])
}
