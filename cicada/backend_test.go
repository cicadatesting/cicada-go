package cicada

import (
	"testing"
	"time"
)

func TestLocalEventBus(t *testing.T) {
	eventBus := NewLocalEventBus()

	subscription := eventBus.Subscribe("test_event")

	eventBus.Emit("test_event", "foo")

	event := <-subscription.events

	if "foo" != event.(string) {
		t.Error("Expected to recieve event")
	}

	eventBus.Unsubscribe(subscription)

	_, isOpen := <-subscription.events

	if isOpen {
		t.Error("Expected events to be closed after unsubscribing")
	}
}

func TestLocalEvents(t *testing.T) {
	scenario := NewScenario("test", func(state *State) (interface{}, error) { return nil, nil }, &ScenarioOptions{})
	eventBus := NewLocalEventBus()

	scenarioBackend := NewLocalScenarioBackend(scenario, eventBus, &State{})
	userBackend := NewLocalUserBackend("123", scenario, eventBus)

	scenarioBackend.numUsers = 1
	scenarioBackend.userIDs = []string{"123"}

	scenarioBackend.SendUserEvents("test", nil)

	events := userBackend.GetUserEvents("test")
	timeout := time.After(time.Second)

	select {
	case event := <-events:
		if event.Event.Kind != "test" {
			t.Error("Expected to recieve event ")
		}
	case <-timeout:
		t.Error("Timeout out")
	}
}
