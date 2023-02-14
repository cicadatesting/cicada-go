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

// Returns channel that recieves test result once test is finished
func (rt *RunningTest) Result() <-chan *PoppedTestResult {
	return rt.backend.GetTestResult()
}

// Returns channel that recieves status updates from test
func (rt *RunningTest) Events() <-chan *PoppedTestEvent {
	return rt.backend.GetTestEvents()
}

// Returns channel that recieves metrics from test
func (rt *RunningTest) Metrics() <-chan *PoppedScenarioMetric {
	return rt.backend.GetTestMetrics()
}
