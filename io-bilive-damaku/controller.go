package main

import (
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/allegro/bigcache/v3"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"strings"
	"time"
)

type DamakuController struct {
	// TODO: Aggregate window, Agent manager, Damaku processor, Storage controller, API plane
	agent     *AgentManager
	storage   *StorageController
	metrics   *MetricsService
	centerCtx *CenterContext
	msgChan   chan *nats.Msg
	eventChan chan any
	dupCache  *bigcache.BigCache
	metaCache *bigcache.BigCache
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
	c.metaCache, err = bigcache.New(ctx.Context, bigcache.Config{
		Shards:           1024,
		LifeWindow:       time.Minute * 30,
		CleanWindow:      time.Hour,
		MaxEntrySize:     500,
		HardMaxCacheSize: 0,
	})
	if err != nil {
		klog.Fatalf("failed to init metaCache: %s", err.Error())
	}
}

// receive agent message and process duplicate, msg subject should be *.agent.*, can be parallelization
func (c *DamakuController) aggregateWindow() {
	c.centerCtx.Worker.Add(1)
	defer c.centerCtx.Worker.Done()
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
				medal := &agent.FansMedalMeta{}
				if err := proto.Unmarshal(msg.Data, medal); err != nil {
					klog.Fatalf("failed to unmarshal agent fans medal: %s", err.Error())
					continue
				}
				// TODO: compare diff
			case "userInfoMeta":
			// standard msg will unmarshal first, then aggregate the message
			case "damaku":
			case "gift":
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
