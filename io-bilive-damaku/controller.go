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
	msgChan        chan *nats.Msg
	eventChan      chan any
	dupCache       *bigcache.BigCache
	userMetaCache  *bigcache.BigCache // UserInfoMeta: uid
	medalMetaCache *bigcache.BigCache // FansMedalMeta: user:rid
}

func (c *DamakuController) Init(ctx *CenterContext) {
	var err error
	c.centerCtx = ctx
	c.msgChan = make(chan *nats.Msg, 100)
	c.eventChan = make(chan any, 200)
	c.dupCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:      1024,
		LifeWindow:  ctx.Config.Controller.DuplicateWindow,
		CleanWindow: time.Minute,
		OnRemove:    c.dupCacheRelease,
		Logger:      klog.NewStandardLogger("INFO"),
	})
	if err != nil {
		klog.Fatalf("failed to init dupCache: %s", err.Error())
	}
	c.userMetaCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:           1024,
		LifeWindow:       time.Minute * 30,
		CleanWindow:      time.Hour,
		MaxEntrySize:     500,
		HardMaxCacheSize: 0,
	})
	if err != nil {
		klog.Fatalf("failed to init userMetaCache: %s", err.Error())
	}
	c.medalMetaCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:           1024,
		LifeWindow:       time.Minute * 30,
		CleanWindow:      time.Hour,
		MaxEntrySize:     500,
		HardMaxCacheSize: 0,
	})
	if err != nil {
		klog.Fatalf("failed to init medalMetaCache: %s", err.Error())
	}
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
	for {
		select {
		case msg := <-c.msgChan:
			subject := strings.Split(msg.Subject, ".")
			switch subject[len(subject)-1] {
			// agent lifecycle msg will provide to agent manager
			case "info":
				info := &agent.AgentInfo{}
				if err := proto.Unmarshal(msg.Data, info); err != nil {
					klog.Fatalf("failed to unmarshal agent info: %s", err.Error())
					continue
				}
				c.agent.OnAgentInfo(info)
			case "status":
				status := &agent.AgentStatus{}
				if err := proto.Unmarshal(msg.Data, status); err != nil {
					klog.Fatalf("failed to unmarshal agent status: %s", err.Error())
					continue
				}
				c.agent.OnAgentStatus(status)
			// meta msg will unmarshal first, then compare diff from cache
			case "fansMedal":
				meta := &agent.FansMedalMeta{}
				if err := proto.Unmarshal(msg.Data, meta); err != nil {
					klog.Fatalf("failed to unmarshal agent fans medal: %s", err.Error())
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
					continue
				}
				var cachedMeta agent.FansMedalMeta
				if err := proto.Unmarshal(cached, &cachedMeta); err != nil {
					klog.Errorf("failed to unmarshal cached medal meta: %s", err.Error())
					continue
				}
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
					klog.Fatalf("failed to unmarshal agent fans medal: %s", err.Error())
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
					continue
				}
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
					klog.Fatalf("failed to unmarshal damaku: %s", err.Error())
					continue
				}
				mask := c.agent.AgentMask(damaku.Meta.Agent)
				if mask == nil {
					continue
				}
				// unique key: damaku roomID:uid:timestamp
				if damaku.Meta.RoomID == nil {
					klog.Warningf("damaku meta room uid is zero")
					continue
				}
				key := fmt.Sprintf("%d:%d:%d", *damaku.Meta.RoomID, damaku.UID, damaku.Meta.TimeStamp)
				if err := msgDupFilter("damaku", key, damaku, mask); err != nil {
					continue
				}
			case "gift":
				gift := &agent.Gift{}
				if err := proto.Unmarshal(msg.Data, gift); err != nil {
					klog.Fatalf("failed to unmarshal gift: %s", err.Error())
					continue
				}
				mask := c.agent.AgentMask(gift.Meta.Agent)
				if mask == nil {
					continue
				}
				// unique key: TID
				key := strconv.FormatUint(gift.TID, 10)
				if err := msgDupFilter("gift", key, gift, mask); err != nil {
					continue
				}
			case "guard":
			case "superChat":
			case "online":
			case "onlineV2":
			}
		case <-c.centerCtx.Context.Done():
			return
		}
	}
}

// pop cached msg info and push msg hit metrics
func (c *DamakuController) dupCacheRelease(key string, entry []byte) {

}
