package realtime

import (
	"sync"

	"github.com/example/taskflow/backend/internal/domain"
)

type Broker struct {
	mu     sync.Mutex
	subs   map[*subscription]struct{}
	closed bool
}

type subscription struct {
	all      bool
	projects map[string]struct{}
	channel  chan domain.TaskEvent
}

func NewBroker() *Broker { return &Broker{subs: make(map[*subscription]struct{})} }

func (b *Broker) Publish(event domain.TaskEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for sub := range b.subs {
		if !sub.all {
			if _, ok := sub.projects[event.ProjectID]; !ok {
				continue
			}
		}
		select {
		case sub.channel <- event:
		default:
			// Keep the stream live for slow clients while retaining the newest state.
			select {
			case <-sub.channel:
			default:
			}
			select {
			case sub.channel <- event:
			default:
			}
		}
	}
}

func (b *Broker) Subscribe(projectIDs []string) (<-chan domain.TaskEvent, func()) {
	projects := make(map[string]struct{}, len(projectIDs))
	for _, id := range projectIDs {
		projects[id] = struct{}{}
	}
	return b.subscribe(&subscription{projects: projects, channel: make(chan domain.TaskEvent, 64)})
}

// SubscribeAll receives events from every project. Consumers must authorize each
// event before exposing it because project memberships can change at runtime.
func (b *Broker) SubscribeAll() (<-chan domain.TaskEvent, func()) {
	return b.subscribe(&subscription{all: true, channel: make(chan domain.TaskEvent, 64)})
}

func (b *Broker) subscribe(sub *subscription) (<-chan domain.TaskEvent, func()) {
	b.mu.Lock()
	if b.closed {
		close(sub.channel)
	} else {
		b.subs[sub] = struct{}{}
	}
	b.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if _, exists := b.subs[sub]; exists {
				delete(b.subs, sub)
				close(sub.channel)
			}
			b.mu.Unlock()
		})
	}
	return sub.channel, cancel
}

// Close terminates every active subscription and prevents future publication or
// subscription. Holding the same mutex as Publish avoids send-on-closed races.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for sub := range b.subs {
		delete(b.subs, sub)
		close(sub.channel)
	}
}
