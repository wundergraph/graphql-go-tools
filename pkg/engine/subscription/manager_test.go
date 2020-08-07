package subscription

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type FakeStream struct {
	done bool
	wg *sync.WaitGroup
}

func (f *FakeStream) Start(input []byte, next chan<- []byte, stop <-chan struct{}) {
	counter := 0
	for {
		select {
		case <-stop:
			f.done = true
			f.wg.Done()
			return
		case <-time.After(time.Duration(1) * time.Millisecond):
			next <- []byte(strconv.Itoa(counter))
			counter++
		}
	}
}

func (f *FakeStream) UniqueIdentifier() []byte {
	return []byte("fake_stream")
}

func TestSubscriptionManager(t *testing.T) {
	fakeStream := &FakeStream{
		wg: &sync.WaitGroup{},
	}
	fakeStream.wg.Add(1)
	manager := NewManager(fakeStream)

	input := []byte("none")

	trigger1, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	trigger2, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	trigger3, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	assert.Equal(t, 3, len(manager.subscriptions[trigger1.SubscriptionID()].triggers))

	receiveOneAndStop := func(trigger *Trigger, wg *sync.WaitGroup, triggerID int) {
		data, ok := trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "0", string(data), "triggerID: %d", triggerID)

		manager.StopTrigger(trigger)

		wg.Done()
	}

	receive := func(trigger *Trigger, wg *sync.WaitGroup, triggerID int) {
		data, ok := trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "0", string(data), "triggerID: %d", triggerID)

		data, ok = trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "1", string(data), "triggerID: %d", triggerID)

		data, ok = trigger.Next(context.Background())
		assert.True(t, ok)
		assert.Equal(t, "2", string(data), "triggerID: %d", triggerID)

		wg.Done()
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go receive(trigger1, wg, 1)
	go receive(trigger2, wg, 2)
	go receiveOneAndStop(trigger3, wg, 3)

	wg.Wait()

	trigger4, err := manager.StartTrigger(input)
	assert.NoError(t, err)

	manager.StopTrigger(trigger1)
	manager.StopTrigger(trigger2)

	data, ok := trigger4.Next(context.Background())
	assert.True(t, ok)
	assert.Equal(t, "3", string(data), "triggerID: %d", 4)

	manager.StopTrigger(trigger4)
	assert.Equal(t, 0, len(manager.subscriptions))
	fakeStream.wg.Wait()
	assert.Equal(t, true, fakeStream.done)
}
