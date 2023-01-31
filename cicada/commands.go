package cicada

import (
	"math"
	"time"
)

type PoppedWork struct {
	Err error
}

type ScenarioCommands interface {
	GetNumUsers() (int, error)
	StartUsers(users int) error
	StopUsers(users int) error
	ScaleUsers(users int) error
	AddWork(amount int) error
	SendUserEvents(kind string, payload interface{}) error
	GetLatestResults() <-chan *PoppedUserResult
	AddDatastoreMetric(name string, value float64) error
	ShutdownChannel() <-chan error
}

type UserCommands interface {
	GetUserID() string
	GetEvents(kind string) <-chan *PoppedUserEvent
	GetWork() <-chan *PoppedWork
	Run(state *State) (interface{}, error)
	ShutdownChannel() <-chan error
	ReportResult(
		output interface{},
		exception error,
		// logs string,
		timeTaken time.Duration,
	) error
}

type LocalScenarioCommands struct {
	scenario *Scenario
	backend  ScenarioBackend
	users    int
}

func NewScenarioCommands(scenario *Scenario, backend ScenarioBackend) *LocalScenarioCommands {
	return &LocalScenarioCommands{
		scenario: scenario,
		users:    0,
		backend:  backend,
	}
}

func (sc *LocalScenarioCommands) StartUsers(users int) error {
	err := sc.backend.StartUsers(users)

	if err != nil {
		return err
	}

	sc.users += users
	return nil
}

func (sc *LocalScenarioCommands) StopUsers(users int) error {
	err := sc.backend.StopUsers(users)

	if err != nil {
		return err
	}

	sc.users -= int(math.Min(float64(users), float64(sc.users)))
	return nil
}

func (sc *LocalScenarioCommands) ScaleUsers(users int) error {
	if users > sc.users {
		return sc.StartUsers(users - sc.users)
	} else {
		return sc.StopUsers(sc.users - users)
	}
}

func (sc *LocalScenarioCommands) GetNumUsers() (int, error) {
	return sc.users, nil
}

func (sc *LocalScenarioCommands) GetLatestResults() <-chan *PoppedUserResult {
	return sc.backend.GetUserResults()
}

func (sc *LocalScenarioCommands) AddDatastoreMetric(name string, value float64) error {
	return sc.backend.AddMetric(name, value)
}

func (sc *LocalScenarioCommands) AddWork(amount int) error {
	return sc.backend.DistributeWork(amount)
}

func (sc *LocalScenarioCommands) SendUserEvents(kind string, payload interface{}) error {
	return sc.backend.SendUserEvents(kind, payload)
}

func (sc *LocalScenarioCommands) ShutdownChannel() <-chan error {
	return sc.backend.ShutdownChannel()
}

type LocalUserCommands struct {
	work     int
	id       string
	backend  UserBackend
	Scenario *Scenario
}

func NewUserCommands(id string, backend UserBackend, scenario *Scenario) *LocalUserCommands {
	return &LocalUserCommands{
		work:     0,
		id:       id,
		backend:  backend,
		Scenario: scenario,
	}
}

func (uc *LocalUserCommands) GetUserID() string {
	return uc.id
}

func (uc *LocalUserCommands) Run(state *State) (interface{}, error) {
	// TODO: capture stdout
	output, err := uc.Scenario.Fn(state)

	return output, err
}

func (uc *LocalUserCommands) GetEvents(kind string) <-chan *PoppedUserEvent {
	return uc.backend.GetUserEvents(kind)
}

func (uc *LocalUserCommands) GetWork() <-chan *PoppedWork {
	poppedWork := make(chan *PoppedWork)
	// NOTE: maybe make this class member?
	workChannel := uc.backend.GetUserEvents("USER_WORK")

	go func() {
		for event := range workChannel {
			amount := event.Event.Payload.(int)

			for i := 0; i < amount; i++ {
				poppedWork <- &PoppedWork{Err: nil}
			}
		}
	}()

	return poppedWork
}

func (uc *LocalUserCommands) ReportResult(
	output interface{},
	exception error,
	// logs string,
	timeTaken time.Duration,
) error {
	result := UserResult{
		Output:    output,
		Exception: exception,
		// Logs:      logs,
		TimeTaken: timeTaken,
	}

	return uc.backend.AddUserResult(&result)
}

func (uc *LocalUserCommands) ShutdownChannel() <-chan error {
	return uc.backend.ShutdownChannel()
}
