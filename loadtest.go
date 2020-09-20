package ltt

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"
)

type loadtestContextKeyType int

var loadtestContextKey loadtestContextKeyType

type StatusType int

var statusTypes = map[StatusType]string{
	StatusStopped:  "stopped",
	StatusSpawning: "spawning",
	StatusRunning:  "running",
	StatusStopping: "stopping",
}

var statusTypesFromString = map[string]StatusType{
	"stopped":  StatusStopped,
	"spawning": StatusSpawning,
	"running":  StatusRunning,
	"stopping": StatusStopping,
}

func (s *StatusType) UnmarshalJSON(bytes []byte) error {
	*s = statusTypesFromString[strings.Trim(string(bytes), "\"")]
	return nil
}

func (s StatusType) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, statusTypes[s])), nil
}

func (s StatusType) String() string {
	return statusTypes[s]
}

const (
	StatusStopped StatusType = iota
	StatusSpawning
	StatusRunning
	StatusStopping
)

type TaskRun struct {
	Task     *Task
	Duration time.Duration
	Error    error
}

type LoadTest struct {
	Config Config     `json:"config"`
	Status StatusType `json:"status"`
	// Map of created users
	UserMap     map[int64]User `json:"-"`
	UserMapLock sync.Mutex     `json:"-"`
	TaskRunChan chan *TaskRun  `json:"-"`
	Stats       *Statistics    `json:"stats"`
	Log         *log.Logger    `json:"-"`
	// Target number of user to spawn
	TargetUserNum int `json:"target_user_num"`
}

func (lt *LoadTest) handleTaskRun(tr *TaskRun) {
	// Only collect stats if we're in a clean running state
	if lt.Status != StatusRunning {
		return
	}

	name := tr.Task.FullName()
	lt.Stats.Lock()
	lt.Stats.RPSMap[time.Now().Unix()]++
	lt.Stats.NumTotal++
	lt.Stats.TotalDuration += tr.Duration.Milliseconds()
	if tr.Error != nil {
		lt.Stats.NumFailed++
	} else {
		lt.Stats.NumSuccessful++
	}

	if _, ok := lt.Stats.Tasks[name]; !ok {
		lt.Stats.Tasks[name] = NewTaskStat(name)
	}

	taskStat := lt.Stats.Tasks[name]
	lt.Stats.Unlock()

	taskStat.Lock()
	durationMS := tr.Duration.Milliseconds()

	taskStat.Metrics[durationMS]++
	taskStat.TotalRuns++
	taskStat.TotalDuration += durationMS
	if tr.Error != nil {
		taskStat.NumFailed++
		taskStat.Errors[tr.Error.Error()]++
	} else {
		taskStat.NumSuccessful++
	}
	taskStat.Unlock()
}

func (lt *LoadTest) usersJob(entryTask *Task) {
	var lastNRU int
	for {
		if lastNRU != lt.Stats.RunningUsers {
			lt.Stats.Lock()
			if lt.Stats.RunningUsers == 0 {
				lt.Status = StatusStopped
				lt.Stats.EndTime = time.Now()
				lt.Log.Printf("All users have been stopped, status changed to stopped\n")
			} else if lt.Stats.RunningUsers == lt.TargetUserNum {
				lt.Status = StatusRunning
				lt.Stats.StartTime = time.Now()
				lt.Log.Printf("All %d users have been spawned, status changed to running\n", lt.TargetUserNum)
			}
			lt.Stats.Unlock()
		}

		lastNRU = lt.Stats.RunningUsers
		lt.UserMapLock.Lock()
		numUsers := len(lt.UserMap)
		lt.UserMapLock.Unlock()

		var diff int
		if numUsers > lt.TargetUserNum {
			if lt.TargetUserNum == 0 {
				lt.Status = StatusStopping
			}
			diff = numUsers - lt.TargetUserNum
			lt.stopUsers(diff)
		} else if numUsers < lt.TargetUserNum {
			lt.Status = StatusSpawning
			diff = lt.TargetUserNum - numUsers
			lt.spawnUsers(diff, entryTask)
		}

		time.Sleep(time.Second)
	}
}

func (lt *LoadTest) stopUsers(num int) {
	defer lt.UserMapLock.Unlock()
	lt.UserMapLock.Lock()

	i := 0
	for _, u := range lt.UserMap {
		if i >= num {
			break
		}
		u.SetStatus(UserStatusStopping)
		i++
	}
}

func (lt *LoadTest) spawnUsers(num int, entryTask *Task) {
	for i := 0; i < num; i++ {
		// Create a new User instance
		var u User
		uv := reflect.ValueOf(lt.Config.UserType)
		if !uv.IsValid() {
			u = NewDefaultUser(entryTask)
		} else {
			ok := false
			u, ok = reflect.New(uv.Type()).Interface().(User)
			if !ok {
				lt.Log.Fatalf("failed to cast LoadTest.User to User\n")
			}
		}

		// Each user has their own context
		ctx := NewLoadTestContext(context.Background(), lt)
		// Save a ref to the user
		ctx = NewUserContext(ctx, u)
		// Setup the user instance's local storage
		ctx = NewStorageContext(ctx, NewStorage())

		u.SetID(int64(i))
		u.SetContext(ctx)

		lt.UserMapLock.Lock()
		lt.UserMap[u.ID()] = u
		lt.UserMapLock.Unlock()

		go func() {
			u.SetStatus(UserStatusSpawning)

			// Sleep according to user ID to ramp up the user spawns
			sleepTime := (int(u.ID()) % lt.TargetUserNum) / lt.Config.NumSpawnPerSecond
			if lt.Config.Verbose {
				lt.Log.Printf("pre-spawn sleep time: %d for user %d\n", sleepTime, u.ID())
			}
			u.SleepSeconds(sleepTime)

			lt.Stats.Lock()
			lt.Stats.RunningUsers++
			lt.Stats.Unlock()

			u.Spawn()

			u.SetStatus(UserStatusRunning)
			for u.Status() == UserStatusRunning {
				u.Tick()
				u.Sleep()
			}

			lt.Stats.Lock()
			lt.Stats.RunningUsers--
			lt.Stats.Unlock()

			lt.UserMapLock.Lock()
			delete(lt.UserMap, u.ID())
			lt.UserMapLock.Unlock()

			u.SetStatus(UserStatusStopped)
		}()
	}
}

func (lt *LoadTest) cleanRPSJob() {
	for {
		lt.Stats.Lock()
		lt.Stats.CleanRPSMap()
		lt.Stats.Unlock()
		time.Sleep(time.Second * 30)
	}
}

func (lt *LoadTest) runAPIJob() {
	err := RunAPIServer(lt)
	if err != nil {
		lt.Log.Println("failed to start api server: %s", err.Error())
	}
}

func (lt *LoadTest) taskRunsJob() {
	for {
		select {
		case tr := <-lt.TaskRunChan:
			lt.handleTaskRun(tr)
		}
	}
}

func (lt *LoadTest) Run(entryTask *Task) {
	lt.Log.Println("Starting Load Testing Tool")

	if lt.Config.SpawnOnStartup {
		lt.TargetUserNum = lt.Config.NumUsers
	}

	go lt.taskRunsJob()
	go lt.runAPIJob()
	go lt.cleanRPSJob()
	go lt.usersJob(entryTask)

	// Run forever
	c := make(chan struct{})
	<-c
}

func NewLoadTest(config Config) *LoadTest {
	return &LoadTest{
		Config:      config,
		Status:      StatusStopped,
		UserMap:     make(map[int64]User, config.NumUsers),
		Stats:       NewStatistics(),
		TaskRunChan: make(chan *TaskRun, config.NumUsers),
		Log:         log.New(config.LogOutput, config.LogPrefix, config.LogFlags),
	}
}

func FromContext(ctx context.Context) *LoadTest {
	if lt, ok := ctx.Value(loadtestContextKey).(*LoadTest); ok {
		return lt
	}

	return nil
}

func NewLoadTestContext(ctx context.Context, lt *LoadTest) context.Context {
	return context.WithValue(ctx, loadtestContextKey, lt)
}
