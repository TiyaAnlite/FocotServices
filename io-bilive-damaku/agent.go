package main

import (
	"encoding/binary"
	"fmt"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/nats-io/nats.go"
	"github.com/zoumo/goset"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"slices"
	"strings"
	"sync"
	"time"
)

type AgentManager struct {
	centerCtx   *CenterContext
	managed     sync.Map // agentId:*AgentStatus
	agentChan   chan *nats.Msg
	roomProvide chan *ProvidedRoom
	roomRevoke  chan uint64
	watchedRoom goset.Set // thread safe, uint64
	latestMask  uint16
	master      *AgentStatus
	mu          sync.RWMutex
}

func (m *AgentManager) Init(ctx *CenterContext) error {
	m.centerCtx = ctx
	m.agentChan = make(chan *nats.Msg, 32)
	m.roomProvide = make(chan *ProvidedRoom)
	m.roomRevoke = make(chan uint64)
	m.watchedRoom = goset.NewSet()
	sub, err := ctx.MQ.Nc.ChanSubscribe(fmt.Sprintf("%s.agent.*", ctx.Config.Global.Prefix), m.agentChan)
	if err != nil {
		return fmt.Errorf("failed to subscribe agent msg: %s", err.Error())
	}
	ctx.MQ.AddSubscribe(sub)
	go m.manager()
	return nil
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
		return
	}
	var hitAgent []uint16
	for i := 0; i < len(masks); i += 2 {
		hitAgent = append(hitAgent, binary.BigEndian.Uint16(masks[i:i+2]))
	}
	m.managed.Range(func(_, value any) bool {
		a := value.(*AgentStatus)
		if !slices.Contains(hitAgent, a.Mask) {
			return true
		}
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.master != nil {
		return ""
	}
	id := m.master.ID
	return id
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
	go m.agentStatus()
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
					if err := m.control(initMsg, "init", a.ID); err != nil {
						klog.Errorf("agent init failed: %s", err.Error())
						return true
					}
					a.mu.Lock()
					a.Condition |= AgentInitialization
					a.UpdateTime = time.Now()
					klog.Infof("agent(%s) condition changed to %s ", a.ID, a.StatusString())
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
			m.managed.Range(func(_, value any) bool {
				a := value.(*AgentStatus)
				var needAdd []uint64
				var needDel []uint64
				a.mu.RLock()
				// only sync ready agent
				if !a.IsReady() {
					a.mu.RUnlock()
					return true
				}
				m.watchedRoom.Range(func(_ int, elem interface{}) bool {
					room := elem.(uint64)
					if !slices.Contains(a.CachedStatus.Watching, room) {
						needAdd = append(needAdd, room)
					}
					return true
				})
				for _, r := range a.CachedStatus.Watching {
					if !m.watchedRoom.Contains(r) {
						needDel = append(needDel, r)
					}
				}
				if a.Condition&AgentSync > 0 && (len(needAdd) > 0 || len(needDel) > 0) {
					// update condition first
					a.mu.RUnlock()
					a.mu.Lock()
					a.Condition ^= AgentSync
					a.UpdateTime = time.Now()
					klog.Infof("agent(%s) condition changed to %s ", a.ID, a.StatusString())
					a.mu.Unlock()
				}
				a.mu.RUnlock()
				// sync diff rooms
				for _, room := range needAdd {
					action := &agent.AgentAction{
						Type:   agent.AgentAction_AddRoom,
						RoomID: &room,
					}
					if err := m.control(action, "action", a.ID); err != nil {
						klog.Errorf("agent add failed: %s", err.Error())
						continue
					}
				}
				for _, room := range needDel {
					action := &agent.AgentAction{
						Type:   agent.AgentAction_DelRoom,
						RoomID: &room,
					}
					if err := m.control(action, "action", a.ID); err != nil {
						klog.Errorf("agent del failed: %s", err.Error())
						continue
					}
				}
				a.mu.Lock()
				a.Condition |= AgentSync
				a.UpdateTime = time.Now()
				klog.Infof("agent(%s) condition changed to %s ", a.ID, a.StatusString())
				a.mu.Unlock()
				// select a master
				if m.master == nil || m.master.Condition&AgentReady == 0 {
					m.mu.Lock()
					m.master = a
					klog.Infof("master agent changed to: %s", a.ID)
					m.mu.Unlock()
				}
				return true
			})
		case <-m.centerCtx.Context.Done():
			return
		}
	}
}

// receive agent msg and update agent status by time
func (m *AgentManager) agentStatus() {
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
				if a.Condition&AgentReady > 0 && time.Now().Sub(a.UpdateTime) > time.Second*3 {
					// agent is no longer ready
					a.mu.RUnlock()
					a.mu.Lock()
					// check again
					if a.Condition&AgentReady > 0 && time.Now().Sub(a.UpdateTime) > time.Second*3 {
						a.Condition = ^AgentReady // unset ready
					}
					a.UpdateTime = time.Now()
					klog.Infof("agent(%s) condition changed to %s ", a.ID, a.StatusString())
					a.mu.Unlock()
					return true
				}
				a.mu.RUnlock()
				return true
			})
		case msg := <-m.agentChan:
			subject := strings.Split(msg.Subject, ".")
			switch subject[len(subject)-1] {
			case "info":
				info := &agent.AgentInfo{}
				if err := proto.Unmarshal(msg.Data, info); err != nil {
					klog.Errorf("failed to unmarshal agent info: %s", err.Error())
					continue
				}
				v, ok := m.managed.Load(info.ID)
				if !ok {
					// auto register new agent
					// create new agent
					m.mu.Lock()
					newAgent := &AgentStatus{
						ID:         info.ID,
						Mask:       m.latestMask,
						UpdateTime: time.Now(),
						HitStatus:  make(map[string]uint32),
					}
					m.latestMask++
					m.mu.Unlock()
					m.managed.Store(newAgent.ID, newAgent)
					klog.Infof("new managered agent: %s", newAgent.ID)
					return
				}
				a := v.(*AgentStatus)
				// info: set agent to not initialized
				a.mu.Lock()
				// reset all condition for restarted agent
				a.Condition = 0
				a.UpdateTime = time.Now()
				klog.Infof("agent(%s) condition changed to %s ", a.ID, a.StatusString())
				a.mu.Unlock()
			case "status":
				status := &agent.AgentStatus{}
				if err := proto.Unmarshal(msg.Data, status); err != nil {
					klog.Fatalf("failed to unmarshal agent status: %s", err.Error())
					continue
				}
				v, ok := m.managed.Load(status.Meta.Agent)
				if !ok {
					// no register agent will be ignored
					return
				}
				a := v.(*AgentStatus)
				// status: set agent to ready
				a.mu.Lock()
				a.CachedStatus = status
				a.Condition |= AgentInitialization | AgentReady // set initialized & ready
				a.UpdateTime = time.Now()
				klog.Infof("agent(%s) condition changed to %s ", a.ID, a.StatusString())
				a.mu.Unlock()
			}
		case <-m.centerCtx.Context.Done():
			return
		}
	}
}

func (m *AgentManager) control(msg proto.Message, action, agentId string) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal agent(%s) %s control failed: %s", agentId, action, err.Error())
	}
	resp, err := m.centerCtx.MQ.Request(
		fmt.Sprintf("%s.agent.%s.%s", m.centerCtx.Config.Global.Prefix, agentId, action),
		payload,
		time.Second*3)
	if err != nil {
		return fmt.Errorf("agent(%s) %s control failed: %s", agentId, action, err.Error())
	}
	r := &agent.AgentControlResponse{}
	if err := proto.Unmarshal(resp.Data, r); err != nil {
		return fmt.Errorf("unmarshal agent(%s) %s control response failed: %s", agentId, action, err.Error())
	}
	if r.Status != agent.AgentControlResponse_OK {
		return fmt.Errorf("agent(%s) %s control failed: %s", agentId, action, *r.Error)
	}
	return nil
}
