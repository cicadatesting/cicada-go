package cicada

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type TestBackend interface {
	StartScenario(scenario *Scenario, state *State) error
	GetScenarioResults() <-chan *PoppedScenarioResult
	SendTestEvent(event *TestEvent) error
	SetTestResult(result *TestResult) error
	Stop()
}

type TestEventBackend interface {
	GetTestEvents() <-chan *PoppedTestEvent
	GetTestResult() <-chan *PoppedTestResult
	GetTestMetrics() <-chan *PoppedScenarioMetric
	Stop()
}

type ScenarioBackend interface {
	StartUsers(users int) error
	StopUsers(users int) error
	DistributeWork(work int) error
	SendUserEvents(kind string, payload interface{}) error
	GetUserResults() <-chan *PoppedUserResult
	SetScenarioResult(output interface{}, exception error, timeTaken time.Duration) error
	AddMetric(name string, value float64) error
	ShutdownChannel() <-chan error
	Stop()
}

type UserBackend interface {
	GetUserEvents(kind string) <-chan *PoppedUserEvent
	AddUserResult(result *UserResult) error
	ShutdownChannel() <-chan error
	Stop()
}

type BackendBuilder interface {
	GetTestBackend() TestBackend
	GetTestEventBackend() TestEventBackend
	GetScenarioBackend(scenario *Scenario, state *State) ScenarioBackend
}

type LocalBackendBuilder struct {
	eventBus *LocalEventBus
}

func NewLocalBackendBuilder() *LocalBackendBuilder {
	return &LocalBackendBuilder{
		eventBus: NewLocalEventBus(),
	}
}

func (b *LocalBackendBuilder) GetTestBackend() TestBackend {
	return NewLocalTestBackend(b.eventBus)
}

func (b *LocalBackendBuilder) GetTestEventBackend() TestEventBackend {
	return NewLocalTestEventBackend(b.eventBus)
}

func (b *LocalBackendBuilder) GetScenarioBackend(scenario *Scenario, state *State) ScenarioBackend {
	return NewLocalScenarioBackend(scenario, b.eventBus, state)
}

type LocalEventBusSubscription struct {
	id     string
	kind   string
	events chan interface{}
	closed bool
	lock   sync.Mutex
}

type LocalEventBus struct {
	lock        sync.Mutex
	subscribers map[string]map[string]*LocalEventBusSubscription
}

func NewLocalEventBus() *LocalEventBus {
	return &LocalEventBus{
		lock:        sync.Mutex{},
		subscribers: map[string]map[string]*LocalEventBusSubscription{},
	}
}

func (b *LocalEventBus) Subscribe(kind string, buffer ...int) *LocalEventBusSubscription {
	id := uuid.NewString()
	size := 0

	if len(buffer) > 0 {
		size = buffer[0]
	}

	subscriber := LocalEventBusSubscription{
		id:     id,
		kind:   kind,
		events: make(chan interface{}, size),
		closed: false,
		lock:   sync.Mutex{},
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	_, ok := b.subscribers[kind]

	if !ok {
		b.subscribers[kind] = map[string]*LocalEventBusSubscription{}
	}

	b.subscribers[kind][id] = &subscriber
	return &subscriber
}

func (b *LocalEventBus) Unsubscribe(subscriber *LocalEventBusSubscription) {
	b.lock.Lock()
	defer b.lock.Unlock()

	subscribers, hasKind := b.subscribers[subscriber.kind]

	if !hasKind {
		return
	}

	sub, hasSub := subscribers[subscriber.id]

	if !hasSub {
		return
	}

	go func() {
		sub.lock.Lock()
		sub.closed = true
		close(sub.events)
		sub.lock.Unlock()

		b.lock.Lock()
		delete(b.subscribers[sub.kind], sub.id)
		b.lock.Unlock()
	}()
}

func (b *LocalEventBus) Emit(kind string, payload interface{}) <-chan bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	doneChan := make(chan bool)
	var wg sync.WaitGroup

	go func() {
		wg.Wait()
		doneChan <- true
	}()

	for _, subscription := range b.subscribers[kind] {
		wg.Add(1)

		go func(s *LocalEventBusSubscription) {
			s.lock.Lock()
			defer s.lock.Unlock()
			defer wg.Done()

			if !s.closed {
				s.events <- payload
			}
		}(subscription)
	}

	return doneChan
}

type LocalTestBackend struct {
	eventBus      *LocalEventBus
	resultChannel chan *PoppedScenarioResult
	// scenarioBackends      map[string]*LocalScenarioBackend
	numWorkers            *int32
	workerShutdownChannel chan bool
}

func NewLocalTestBackend(eventBus *LocalEventBus) *LocalTestBackend {
	var numWorkers int32

	return &LocalTestBackend{
		eventBus:              eventBus,
		resultChannel:         make(chan *PoppedScenarioResult),
		numWorkers:            &numWorkers,
		workerShutdownChannel: make(chan bool),
		// scenarioBackends:      map[string]*LocalScenarioBackend{},
	}
}

func (b *LocalTestBackend) StartScenario(scenario *Scenario, state *State) error {
	atomic.AddInt32(b.numWorkers, 1)

	go func() {
		backend := NewLocalScenarioBackend(scenario, b.eventBus, state)
		subscriber := b.eventBus.Subscribe(fmt.Sprintf("SCENARIO_%s_RESULT", scenario.Name))

		scenarioRunner(scenario, backend, state)

		select {
		case result := <-subscriber.events:
			b.resultChannel <- &PoppedScenarioResult{Result: result.(*ScenarioResult), Err: nil}
		case <-b.workerShutdownChannel:
			break
		}

		atomic.AddInt32(b.numWorkers, -1)
		backend.Stop()
	}()

	return nil
}

func (b *LocalTestBackend) GetScenarioResults() <-chan *PoppedScenarioResult {
	return b.resultChannel
}

func (b *LocalTestBackend) SendTestEvent(event *TestEvent) error {
	b.eventBus.Emit("TEST_EVENT", event)

	return nil
}

func (b *LocalTestBackend) SetTestResult(result *TestResult) error {
	doneChan := b.eventBus.Emit("TEST_RESULT", result)

	<-doneChan
	return nil
}

func (b *LocalTestBackend) Stop() {
	for i := 0; i < int(*b.numWorkers); i++ {
		b.workerShutdownChannel <- false
	}
}

type LocalTestEventBackend struct {
	testEventChan        chan *PoppedTestEvent
	testResultChan       chan *PoppedTestResult
	scenarioMetricChan   chan *PoppedScenarioMetric
	testEventSubscriber  *LocalEventBusSubscription
	testResultSubscriber *LocalEventBusSubscription
	testMetricSubscriber *LocalEventBusSubscription
	eventBus             *LocalEventBus
}

func NewLocalTestEventBackend(eventBus *LocalEventBus) *LocalTestEventBackend {
	backend := LocalTestEventBackend{
		testEventChan:        make(chan *PoppedTestEvent),
		testResultChan:       make(chan *PoppedTestResult),
		scenarioMetricChan:   make(chan *PoppedScenarioMetric),
		testEventSubscriber:  eventBus.Subscribe("TEST_EVENT"),
		testResultSubscriber: eventBus.Subscribe("TEST_RESULT"),
		testMetricSubscriber: eventBus.Subscribe("TEST_METRIC"),
		eventBus:             eventBus,
	}

	go func() {
		for event := range backend.testEventSubscriber.events {
			backend.testEventChan <- &PoppedTestEvent{
				Event: event.(*TestEvent),
				Err:   nil,
			}
		}

		close(backend.testEventChan)
	}()

	go func() {
		for event := range backend.testResultSubscriber.events {
			backend.testResultChan <- &PoppedTestResult{
				Result: event.(*TestResult),
				Err:    nil,
			}
		}

		close(backend.testResultChan)
	}()

	go func() {
		for event := range backend.testMetricSubscriber.events {
			backend.scenarioMetricChan <- &PoppedScenarioMetric{
				Metric: event.(*ScenarioMetric),
				Err:    nil,
			}
		}

		close(backend.scenarioMetricChan)
	}()

	return &backend
}

func (b *LocalTestEventBackend) GetTestEvents() <-chan *PoppedTestEvent {
	return b.testEventChan
}

func (b *LocalTestEventBackend) GetTestResult() <-chan *PoppedTestResult {
	return b.testResultChan
}

func (b *LocalTestEventBackend) GetTestMetrics() <-chan *PoppedScenarioMetric {
	return b.scenarioMetricChan
}

func (b *LocalTestEventBackend) Stop() {
	b.eventBus.Unsubscribe(b.testEventSubscriber)
	b.eventBus.Unsubscribe(b.testMetricSubscriber)
	b.eventBus.Unsubscribe(b.testResultSubscriber)
}

type LocalScenarioBackend struct {
	scenario                *Scenario
	state                   *State
	numUsers                int
	userIDs                 []string
	eventBuffer             []*Event
	workBuffer              int
	userShutdownChannels    map[string]chan bool
	userResultSubscription  *LocalEventBusSubscription
	eventBus                *LocalEventBus
	numWorkers              *uint32
	scenarioShutdownChannel chan error
	workerShutdownChannels  chan bool
	running                 bool
}

func NewLocalScenarioBackend(
	scenario *Scenario,
	eventBus *LocalEventBus,
	state *State,
) *LocalScenarioBackend {
	var numWorkers uint32

	backend := &LocalScenarioBackend{
		scenario:                scenario,
		state:                   state,
		numUsers:                0,
		eventBuffer:             []*Event{},
		workBuffer:              0,
		userIDs:                 []string{},
		userShutdownChannels:    make(map[string]chan bool),
		userResultSubscription:  eventBus.Subscribe(fmt.Sprintf("%s_USER_RESULTS", scenario.Name), 1000),
		eventBus:                eventBus,
		numWorkers:              &numWorkers,
		scenarioShutdownChannel: make(chan error),
		workerShutdownChannels:  make(chan bool),
		running:                 true,
	}

	return backend
}

func (b *LocalScenarioBackend) StartUsers(users int) error {
	for i := 0; i < users; i++ {
		userID := fmt.Sprintf("cicada-user-%s", uuid.NewString()[:8])

		userBackend := NewLocalUserBackend(userID, b.scenario, b.eventBus)

		b.userIDs = append(b.userIDs, userID)
		b.userShutdownChannels[userID] = make(chan bool)

		go func(shutdownChan <-chan bool) {
			go userRunner(userID, b.scenario, b.state, userBackend)
			<-shutdownChan
			userBackend.Stop()
		}(b.userShutdownChannels[userID])
	}

	b.numUsers += users
	// distribute buffered work
	if b.workBuffer > 0 {
		b.DistributeWork(b.workBuffer)
	}

	// distribute bufferered user events
	for _, event := range b.eventBuffer {
		b.sendUserEvents(event)
	}

	return nil
}

func (b *LocalScenarioBackend) StopUsers(users int) error {
	numUsersToRemove := math.Min(float64(users), float64(b.numUsers))

	usersToRemove := b.userIDs[:int(numUsersToRemove)]
	remainingUsers := b.userIDs[int(numUsersToRemove):]

	for _, userID := range usersToRemove {
		// b.userBackends[userID].Stop()
		b.userShutdownChannels[userID] <- false
		delete(b.userShutdownChannels, userID)
	}

	b.userIDs = remainingUsers
	b.numUsers -= int(numUsersToRemove)

	return nil
}

func (b *LocalScenarioBackend) sendUserEvent(userID string, event *Event) {
	b.eventBus.Emit(fmt.Sprintf("%s_USER_EVENTS", userID), event)
}

func (b *LocalScenarioBackend) sendUserEvents(event *Event) {
	for _, userID := range b.userIDs {
		b.sendUserEvent(userID, event)
	}
}

func (b *LocalScenarioBackend) SendUserEvents(kind string, payload interface{}) error {
	event := Event{
		Kind:    kind,
		Payload: payload,
	}

	if b.numUsers < 1 {
		b.eventBuffer = append(b.eventBuffer, &event)
		return nil
	}

	b.sendUserEvents(&event)
	return nil
}

func (b *LocalScenarioBackend) DistributeWork(work int) error {
	if b.numUsers < 1 {
		b.workBuffer += work
		return nil
	}

	// per user work is work / number of users
	baseWork := work / b.numUsers
	remainingWork := work % b.numUsers
	workPerUser := map[string]int{}

	// distribute work evenly to all users for per user work
	if baseWork > 0 {
		for _, userID := range b.userIDs {
			workPerUser[userID] = baseWork
		}
	}

	// for remaining work (work % number of users) distribute 1 unit of work to randomly selected users
	if remainingWork > 0 {
		rand.Seed(time.Now().Unix())
		rand.Shuffle(b.numUsers, func(i, j int) {
			b.userIDs[i], b.userIDs[j] = b.userIDs[j], b.userIDs[i]
		})

		for i := 0; i < remainingWork; i++ {
			userID := b.userIDs[i]
			currentWork, hasCurrentWork := workPerUser[userID]

			if !hasCurrentWork {
				workPerUser[userID] = 1
			} else {
				workPerUser[userID] = currentWork + 1
			}
		}
	}

	// send events to users with work
	for userID, amount := range workPerUser {
		event := Event{
			Kind:    "USER_WORK",
			Payload: amount,
		}

		b.sendUserEvent(userID, &event)
	}

	return nil
}

func (b *LocalScenarioBackend) GetUserResults() <-chan *PoppedUserResult {
	userResults := make(chan *PoppedUserResult)
	atomic.AddUint32(b.numWorkers, 1)

	go func() {
		for {
			select {
			case event := <-b.userResultSubscription.events:
				userResults <- &PoppedUserResult{
					Result: event.(*UserResult),
					Err:    nil,
				}
			case <-b.workerShutdownChannels:
				// close(userResults)
				return
			}
		}
	}()

	return userResults
}

func (b *LocalScenarioBackend) ShutdownChannel() <-chan error {
	return b.scenarioShutdownChannel
}

func (b *LocalScenarioBackend) AddMetric(name string, value float64) error {
	b.eventBus.Emit("TEST_METRIC", &ScenarioMetric{
		Scenario: b.scenario.Name,
		Name:     name,
		Value:    value,
	})

	return nil
}

func (b *LocalScenarioBackend) SetScenarioResult(output interface{}, exception error, timeTaken time.Duration) error {
	b.eventBus.Emit(fmt.Sprintf("SCENARIO_%s_RESULT", b.scenario.Name), &ScenarioResult{
		ScenarioName: b.scenario.Name,
		Result:       output,
		Exception:    exception,
		TimeTaken:    timeTaken,
	})

	return nil
}

func (b *LocalScenarioBackend) Stop() {
	b.scenarioShutdownChannel <- nil
	b.running = false
	b.StopUsers(b.numUsers)

	for i := 0; i < int(*b.numWorkers); i++ {
		b.workerShutdownChannels <- false
	}

	b.eventBus.Unsubscribe(b.userResultSubscription)
}

type LocalUserBackend struct {
	userID                string
	scenario              *Scenario
	eventBus              *LocalEventBus
	eventSubscription     *LocalEventBusSubscription
	eventChannelBuffers   map[string]chan *PoppedUserEvent
	userLock              sync.Mutex
	userShutdownChannel   chan error
	workerShutdownChannel chan bool
	running               bool
}

func NewLocalUserBackend(userID string, scenario *Scenario, eventBus *LocalEventBus) *LocalUserBackend {
	backend := LocalUserBackend{
		userID:                userID,
		scenario:              scenario,
		eventBus:              eventBus,
		eventSubscription:     eventBus.Subscribe(fmt.Sprintf("%s_USER_EVENTS", userID)),
		eventChannelBuffers:   map[string]chan *PoppedUserEvent{},
		userLock:              sync.Mutex{},
		userShutdownChannel:   make(chan error),
		workerShutdownChannel: make(chan bool),
		running:               true,
	}

	go func() {
		for {
			select {
			case event := <-backend.eventSubscription.events:
				userEvent := event.(*Event)

				backend.userLock.Lock()
				buffer, hasBuffer := backend.eventChannelBuffers[userEvent.Kind]

				if !hasBuffer {
					backend.eventChannelBuffers[userEvent.Kind] = make(chan *PoppedUserEvent)
					buffer = backend.eventChannelBuffers[userEvent.Kind]
				}

				backend.userLock.Unlock()

				go func() {
					backend.userLock.Lock()
					defer backend.userLock.Unlock()

					if backend.running {
						buffer <- &PoppedUserEvent{
							Event: userEvent,
							Err:   nil,
						}
					}
				}()
			case <-backend.workerShutdownChannel:
				return
			}
		}
	}()

	return &backend
}

func (b *LocalUserBackend) AddUserResult(result *UserResult) error {
	b.eventBus.Emit(fmt.Sprintf("%s_USER_RESULTS", b.scenario.Name), result)
	return nil
}

func (b *LocalUserBackend) GetUserEvents(kind string) <-chan *PoppedUserEvent {
	buffer, hasBuffer := b.eventChannelBuffers[kind]

	if !hasBuffer {
		b.userLock.Lock()
		b.eventChannelBuffers[kind] = make(chan *PoppedUserEvent)
		buffer = b.eventChannelBuffers[kind]
		b.userLock.Unlock()
	}

	return buffer
}

func (b *LocalUserBackend) ShutdownChannel() <-chan error {
	return b.userShutdownChannel
}

func (b *LocalUserBackend) Stop() {
	b.userLock.Lock()
	defer b.userLock.Unlock()

	b.userShutdownChannel <- nil
	b.workerShutdownChannel <- true
	b.eventBus.Unsubscribe(b.eventSubscription)
	b.running = false

	// for _, buffer := range b.eventChannelBuffers {
	// 	close(buffer)
	// }
}
