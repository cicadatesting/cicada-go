package cicada

type Event struct {
	Kind    string
	Payload interface{}
}

type TestEvent struct {
	Text             string
	ScenarioStatuses map[string]ScenarioStatus
}

type PoppedTestEvent struct {
	Event *TestEvent
	Err   error
}

type PoppedUserEvent struct {
	Event *Event
	Err   error
}

type RunningTest struct {
	backend            TestEventBackend
	recievedTestEvents []*TestEvent
}

func NewRunningTest(backend TestEventBackend) *RunningTest {
	runningTest := RunningTest{
		backend:            backend,
		recievedTestEvents: []*TestEvent{},
	}

	return &runningTest
}

func (rt *RunningTest) Result() <-chan *PoppedTestResult {
	return rt.backend.GetTestResult()
}

func (rt *RunningTest) Events() <-chan *PoppedTestEvent {
	return rt.backend.GetTestEvents()
}

func (rt *RunningTest) Metrics() <-chan *PoppedScenarioMetric {
	return rt.backend.GetTestMetrics()
}
