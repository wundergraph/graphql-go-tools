package subscription

import (
	"sync"

	"github.com/jensneuse/graphql-go-tools/pkg/pool"
)

func NewManager(stream Stream) *Manager {
	return &Manager{
		stream:        stream,
		subscriptions: map[uint64]*sub{},
	}
}

type Manager struct {
	subscriptions map[uint64]*sub
	stream        Stream
	mux           sync.RWMutex
}

type sub struct {
	sync.RWMutex
	triggers []*Trigger
}

func (m *Manager) StartTrigger(input []byte) (trigger *Trigger, err error) {

	subscriptionID := m.subscriptionID(input)

	trigger = NewTrigger(subscriptionID)

	m.mux.Lock()
	defer m.mux.Unlock()

	subs, ok := m.subscriptions[subscriptionID]
	if ok {
		subs.Lock()
		subs.triggers = append(subs.triggers, trigger)
		subs.Unlock()
		return
	}

	m.subscriptions[subscriptionID] = &sub{
		triggers: []*Trigger{trigger},
	}

	go m.startPolling(subscriptionID, input)

	return
}

func (m *Manager) StopTrigger(trigger *Trigger) {
	subscriptionID := trigger.SubscriptionID()
	m.mux.Lock()
	defer m.mux.Unlock()
	subs, ok := m.subscriptions[subscriptionID]
	if !ok {
		return
	}
	subs.Lock()
	defer subs.Unlock()
	subs.triggers = m.deleteTrigger(m.subscriptions[subscriptionID].triggers, trigger)

	if len(subs.triggers) == 0 {
		delete(m.subscriptions, subscriptionID)
		return
	} else {
		m.subscriptions[subscriptionID] = subs
	}
}

func (m *Manager) deleteTrigger(triggers []*Trigger, trigger *Trigger) []*Trigger {
	i := m.triggerIndex(triggers, trigger)
	if i == -1 {
		return triggers
	}
	copy(triggers[i:], triggers[i+1:])
	triggers[len(triggers)-1] = nil
	triggers = triggers[:len(triggers)-1]
	return triggers
}

func (m *Manager) triggerIndex(triggers []*Trigger, trigger *Trigger) int {
	for i := range triggers {
		if triggers[i] == trigger {
			return i
		}
	}
	return -1
}

func (m *Manager) startPolling(subscriptionID uint64, input []byte) {
	stop := make(chan struct{})
	next := make(chan []byte)
	go func() {
		m.stream.Start(input, next, stop)
	}()
	for {
		select {
		case message := <-next:
			m.mux.RLock()
			subs, ok := m.subscriptions[subscriptionID]
			m.mux.RUnlock()
			if !ok {
				stop <- struct{}{}
				return
			}
			subs.RLock()
			for i := range subs.triggers {
				select {
				case subs.triggers[i].results <- message:
				default:
					continue
				}
			}
			subs.RUnlock()
		default:
			m.mux.RLock()
			_, ok := m.subscriptions[subscriptionID]
			m.mux.RUnlock()
			if ok {
				continue
			}
			stop <- struct{}{}
			return
		}
	}
}

func (m *Manager) subscriptionID(input []byte) uint64 {
	hash64 := pool.Hash64.Get()
	_, _ = hash64.Write(input)
	subscriptionID := hash64.Sum64()
	pool.Hash64.Put(hash64)
	return subscriptionID
}

func (m *Manager) UniqueIdentifier() []byte {
	return m.stream.UniqueIdentifier()
}
