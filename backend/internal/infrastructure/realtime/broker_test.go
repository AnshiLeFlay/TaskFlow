package realtime

import (
	"sync"
	"testing"

	"github.com/example/taskflow/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrokerFiltersScopedSubscriptionsAndSupportsSubscribeAll(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)

	projectEvents, unsubscribeProject := broker.Subscribe([]string{"project-a"})
	defer unsubscribeProject()
	allEvents, unsubscribeAll := broker.SubscribeAll()
	defer unsubscribeAll()

	projectB := domain.TaskEvent{ID: "event-b", ProjectID: "project-b"}
	broker.Publish(projectB)
	assertChannelEmpty(t, projectEvents)
	assert.Equal(t, projectB, receiveEvent(t, allEvents))

	projectA := domain.TaskEvent{ID: "event-a", ProjectID: "project-a"}
	broker.Publish(projectA)
	assert.Equal(t, projectA, receiveEvent(t, projectEvents))
	assert.Equal(t, projectA, receiveEvent(t, allEvents))
}

func TestBrokerCloseTerminatesCurrentAndFutureSubscriptions(t *testing.T) {
	broker := NewBroker()
	current, unsubscribe := broker.SubscribeAll()

	broker.Close()
	broker.Close()
	unsubscribe()
	broker.Publish(domain.TaskEvent{ProjectID: "project-a"})

	_, ok := <-current
	assert.False(t, ok)
	future, cancelFuture := broker.Subscribe([]string{"project-a"})
	defer cancelFuture()
	_, ok = <-future
	assert.False(t, ok)
}

func TestBrokerCloseIsSafeDuringPublishAndUnsubscribe(t *testing.T) {
	broker := NewBroker()
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		_, unsubscribe := broker.SubscribeAll()
		wait.Add(2)
		go func() {
			defer wait.Done()
			for j := 0; j < 100; j++ {
				broker.Publish(domain.TaskEvent{ProjectID: "project"})
			}
		}()
		go func() {
			defer wait.Done()
			unsubscribe()
		}()
	}
	broker.Close()
	wait.Wait()
}

func receiveEvent(t *testing.T, events <-chan domain.TaskEvent) domain.TaskEvent {
	t.Helper()
	event, ok := <-events
	require.True(t, ok)
	return event
}

func assertChannelEmpty(t *testing.T, events <-chan domain.TaskEvent) {
	t.Helper()
	select {
	case event, ok := <-events:
		require.True(t, ok)
		t.Fatalf("unexpected event: %+v", event)
	default:
	}
}
