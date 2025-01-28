package main

import (
	"encoding/binary"
	"fmt"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/zoumo/goset"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

type AgentManager struct {
	centerCtx   *CenterContext
	managed     sync.Map // agentId:*AgentStatus
	roomProvide chan *ProvidedRoom
	roomRevoke  chan uint64
	watchedRoom goset.Set
	latestMask  uint16
	master      *AgentStatus
}

func (m *AgentManager) Init(ctx *CenterContext) {
	m.centerCtx = ctx
	m.roomProvide = make(chan *ProvidedRoom)
	m.roomRevoke = make(chan uint64)
	m.watchedRoom = goset.NewSet()
	go m.manager()
}

func (m *AgentManager) OnAgentInfo(info *agent.AgentInfo) {
	v, ok := m.managed.Load(info.ID)
	if !ok {
		return
	}
	a := v.(*AgentStatus)
	// info: set agent to not initialized
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Condition&AgentInitialization > 0 {
		a.Condition = ^AgentInitialization // unset initialization
	}
	a.UpdateTime = time.Now()
}

func (m *AgentManager) OnAgentStatus(status *agent.AgentStatus) {
	v, ok := m.managed.Load(status.Meta.Agent)
	if !ok {
		return
	}
	a := v.(*AgentStatus)
	// status: set agent to ready
	a.mu.Lock()
	defer a.mu.Unlock()
	a.CachedStatus = status
	a.Condition = a.Condition | AgentInitialization | AgentReady
	a.UpdateTime = time.Now()
}

// AgentMask return unique agent binary mask using for cache or identifier
// if agent not exist, will return nil
func (m *AgentManager) AgentMask(agentId string) []byte {
	if v, ok := m.managed.Load(agentId); ok {
		a := v.(*AgentStatus)
		mask := make([]byte, 2)
		binary.BigEndian.PutUint16(mask, a.Mask)
		return mask
	} else {
		return nil
	}
}

// AddAgentHitMask will update agent message counter
// the masks must a serial of AgentMask
func (m *AgentManager) AddAgentHitMask(masks []byte, category string) {
	if len(masks)%2 != 0 {
		klog.Warningf("illegal mask length: %d", len(masks))
	}
	var hitAgent []uint16
	for i := 0; i < len(masks); i += 2 {
		hitAgent = append(hitAgent, binary.BigEndian.Uint16(masks[i:i+2]))
	}
	m.managed.Range(func(_, value any) bool {
		a := value.(*AgentStatus)
		a.mu.Lock()
		defer a.mu.Unlock()
		if _, ok := a.HitStatus[category]; ok {
			a.HitStatus[category] += 1
		} else {
			a.HitStatus[category] = 1
		}
		return true
	})
}

// MasterAgent return the first choice agent for single stream
func (m *AgentManager) MasterAgent() string {
	if m.master != nil {
		return ""
	}
	return m.master.ID
}

// GetRoomChan get two channels for provide and revoke rooms
func (m *AgentManager) GetRoomChan() (chan<- *ProvidedRoom, chan<- uint64) {
	return m.roomProvide, m.roomRevoke
}

func (m *AgentManager) manager() {
	m.centerCtx.Worker.Add(1)
	defer m.centerCtx.Worker.Done()
	klog.Info("agent manager started")
	go m.initAgent()
	go m.syncAgent()
}

// init no initialization agent
func (m *AgentManager) initAgent() {
	m.centerCtx.Worker.Add(1)
	defer m.centerCtx.Worker.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.managed.Range(func(_, value any) bool {
				a := value.(*AgentStatus)
				a.mu.RLock()
				if a.Condition&AgentInitialization == 0 && time.Now().Sub(a.UpdateTime) <= time.Second*3 {
					// init agent async
					a.mu.RUnlock()
					initMsg := &agent.AgentInit{
						BUVID:  m.centerCtx.Config.Global.BUVID,
						UID:    m.centerCtx.Config.Global.UID,
						Cookie: m.centerCtx.Config.Global.Cookie,
					}
					initData, err := proto.Marshal(initMsg)
					if err != nil {
						klog.Warningf("marshal init agent init failed: %s", err.Error())
						return true
					}
					msg, err := m.centerCtx.MQ.Request(
						fmt.Sprintf("%s.agent.%s.init", m.centerCtx.Config.Global.Prefix, a.ID),
						initData,
						time.Second*3,
					)
					if err != nil {
						klog.Errorf("agent init failed: %s", err.Error())
						return true
					}
					resp := &agent.AgentControlResponse{}
					if err := proto.Unmarshal(msg.Data, resp); err != nil {
						klog.Errorf("agent init failed: %s", err.Error())
						return true
					}
					if resp.Status != agent.AgentControlResponse_OK {
						klog.Errorf("agent init failed: %s", *resp.Error)
						return true
					}
					a.mu.Lock()
					a.Condition = a.Condition | AgentInitialization
					a.UpdateTime = time.Now()
					a.mu.Unlock()
					return true
				}
				a.mu.RUnlock()
				return true
			})
		case <-m.centerCtx.Context.Done():
			return
		}
	}
}

// sync need sync agent
func (m *AgentManager) syncAgent() {
	m.centerCtx.Worker.Add(1)
	defer m.centerCtx.Worker.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-m.centerCtx.Context.Done():
			return
		}
	}
}
