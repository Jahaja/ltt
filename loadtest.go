package ltt

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"reflect"
	"sort"
	"sync"
	"time"
)

type loadtestContextKeyType int

var loadtestContextKey loadtestContextKeyType

type Config struct {
	// Host to bind the REST API to. default all (empty string).
	APIHost string
	// Port to bind the REST API to, default 4141
	APIPort  int
	NumUsers int
	// How many users to spawn each second
	NumSpawnPerSecond int
	// Default 10 seconds
	RequestTimeout time.Duration
	// Custom user type to override the DefaultUser
	UserType User
	// Min sleep time between tasks in seconds
	MinSleepTime int
	// Max sleep time between tasks in seconds
	MaxSleepTime int
	// Verbose logging
	Verbose bool
	// If we should start spawning users on startup
	SpawnOnStartup bool
	// Logging params
	LogOutput io.Writer
	LogPrefix string
	LogFlags  int
}

type StatusType int

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

type TaskStats struct {
	sync.Mutex
	TotalRuns       int64            `json:"total_runs"`
	NumSuccessful   int64            `json:"num_successful"`
	NumFailed       int64            `json:"num_failed"`
	TotalDuration   int64            `json:"total_duration"`
	Metrics         map[int64]int64  `json:"-"`
	Percentiles     map[int]int64    `json:"percentiles"`
	AverageDuration float32          `json:"average_duration"`
	Errors          map[string]int64 `json:"errors"`
}

func (ts *TaskStats) Calculate() {
	const MinRunsToCalculate = 10
	if ts.TotalRuns < MinRunsToCalculate {
		return
	}

	type flatTaskStats struct {
		Duration int64
		Count    int64
	}

	flatMetrics := make([]flatTaskStats, len(ts.Metrics))
	for d, c := range ts.Metrics {
		flatMetrics = append(flatMetrics, flatTaskStats{d, c})
	}

	sort.Slice(flatMetrics, func(i, j int) bool {
		return flatMetrics[i].Duration < flatMetrics[j].Duration
	})

	percentiles := []float64{0.75, 0.85, 0.95, 0.99}
	for _, p := range percentiles {
		idx := int64(float64(ts.TotalRuns) * p)

		var i int64
		for _, fm := range flatMetrics {
			i += fm.Count

			if i >= idx {
				ts.Percentiles[int(p*100)] = fm.Duration
				break
			}
		}
	}

	ts.AverageDuration = float32(ts.TotalDuration) / float32(ts.TotalRuns)
}

func NewTaskStat() *TaskStats {
	return &TaskStats{
		Metrics:     make(map[int64]int64),
		Percentiles: make(map[int]int64),
		Errors:      make(map[string]int64),
	}
}

type Statistics struct {
	sync.Mutex
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	NumTotal      int64     `json:"num_total"`
	NumUsers      int       `json:"num_users"`
	NumSuccessful int64     `json:"num_successful"`
	NumFailed     int64     `json:"num_failed"`
	TotalDuration int64     `json:"total_duration"`
	// unix-timestamp -> count map to calculate a current RPS value
	RPSMap map[int64]int64 `json:"-"`

	Tasks           map[string]*TaskStats `json:"tasks"`
	CurrentRPS      float32               `json:"current_rps"`
	AverageDuration float32               `json:"average_duration"`
}

const RPSTimeWindow = 10

func (ts *Statistics) Reset() {
	ts.StartTime = time.Now()
	ts.NumTotal = 0
	ts.NumSuccessful = 0
	ts.NumFailed = 0
	ts.TotalDuration = 0
	ts.RPSMap = map[int64]int64{}
	ts.Tasks = map[string]*TaskStats{}
	ts.CurrentRPS = 0
	ts.AverageDuration = 0
}

func (ts *Statistics) CleanRPSMap() {
	now := time.Now().Unix()

	var count int64
	for uts := range ts.RPSMap {
		if uts < now-RPSTimeWindow {
			delete(ts.RPSMap, uts)
		}
	}

	ts.CurrentRPS = float32(count) / float32(RPSTimeWindow)
}

func (ts *Statistics) Calculate() {
	const MinRunsToCalculate = 10
	if ts.NumTotal < MinRunsToCalculate {
		return
	}

	now := time.Now().Unix()

	var count int64
	for uts, c := range ts.RPSMap {
		if uts >= now-RPSTimeWindow {
			count += c
		}
	}

	ts.CurrentRPS = float32(count) / float32(RPSTimeWindow)
	ts.AverageDuration = float32(ts.TotalDuration) / float32(ts.NumTotal)

	for _, t := range ts.Tasks {
		t.Calculate()
	}
}

type LoadTest struct {
	Config Config
	Status StatusType

	Users          map[int64]User
	StatusChan     chan StatusType
	UserStatusChan chan User
	TaskRunChan    chan *TaskRun
	Stats          Statistics
	Log            *log.Logger
}

func NewConfigFromFlags() Config {
	var req_timeout int
	conf := Config{}

	flag.IntVar(&conf.NumUsers, "num-users", 5, "Number of users to spawn")
	flag.IntVar(&req_timeout, "request-timeout", 5000, "Request timeout in ms (Default 5000)")
	flag.IntVar(&conf.MinSleepTime, "min-sleep-time", 1, "Minimum sleep time between a user's tasks in seconds (Default 1)")
	flag.IntVar(&conf.MaxSleepTime, "max-sleep-time", 10, "Maximum sleep time between a user's tasks in seconds (Default 10)")
	flag.IntVar(&conf.NumSpawnPerSecond, "num-spawn-per-sec", 1, "Number of user to spawn per second (Default 1)")
	flag.StringVar(&conf.APIHost, "api-host", "", "REST API port to bind to. (Default all, empty string)")
	flag.StringVar(&conf.LogPrefix, "log-prefix", "", "Logging prefix (Default empty string)")
	flag.IntVar(&conf.APIPort, "api-port", 4141, "REST API port to bind to. (Default 4141)")
	flag.BoolVar(&conf.Verbose, "verbose", false, "Verbose logging (default false)")
	flag.BoolVar(&conf.SpawnOnStartup, "spawn-on-startup", false, "If true, spawning will begin on startup (Default false)")
	flag.Parse()

	conf.RequestTimeout = time.Millisecond * time.Duration(req_timeout)
	if conf.LogOutput == nil {
		conf.LogOutput = os.Stdout
	}

	return conf
}

func (lt *LoadTest) SetStatus(status StatusType) {
	lt.StatusChan <- status
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
		lt.Stats.Tasks[name] = NewTaskStat()
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

func (lt *LoadTest) spawnUsers(entryTask *Task) {
	for i := 0; i < lt.Config.NumUsers; i++ {
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

		go func() {
			// Sleep according to user ID to ramp up the user spawns
			preSpawnSleepTime := u.ID() / int64(lt.Config.NumSpawnPerSecond)
			time.Sleep(time.Second * time.Duration(preSpawnSleepTime))

			u.Spawn()

			u.SetStatus(UserStatusRunning)
			lt.UserStatusChan <- u

			for lt.Status != StatusStopping {
				u.Tick()
				u.Sleep()
			}

			u.SetStatus(UserStatusStopped)
			lt.UserStatusChan <- u
		}()
	}
}

func (lt *LoadTest) handleStatus(status StatusType, entryTask *Task) {
	switch status {
	case StatusSpawning:
		if lt.Status != StatusStopped {
			lt.Log.Printf("invalid state change from %d to %d\n", lt.Status, status)
			return
		}
	case StatusStopping:
		if lt.Status != StatusRunning {
			lt.Log.Printf("invalid state change from %d to %d\n", lt.Status, status)
			return
		}
	}

	lt.Status = status
	if lt.Status == StatusSpawning {
		lt.spawnUsers(entryTask)
	}
}

func (lt *LoadTest) handleUserStatus(u User) {
	if u.Status() == UserStatusRunning {
		lt.Users[u.ID()] = u
	} else if u.Status() == UserStatusStopped {
		delete(lt.Users, u.ID())
	}

	lt.Stats.Lock()
	lt.Stats.NumUsers = len(lt.Users)
	lt.Stats.Unlock()

	if lt.Status == StatusSpawning && len(lt.Users) == lt.Config.NumUsers {
		lt.Stats.Lock()
		lt.Stats.StartTime = time.Now()
		lt.Stats.Unlock()
		lt.Status = StatusRunning
		lt.Log.Printf("All %d users have been spawned, status changed to running\n", lt.Config.NumUsers)
	}

	if lt.Status == StatusStopping && len(lt.Users) == 0 {
		lt.Status = StatusStopped
		lt.Stats.Lock()
		lt.Stats.EndTime = time.Now()
		lt.Stats.Unlock()
		lt.Log.Printf("All users have been stopped, status changed to stopped\n")
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

func (lt *LoadTest) handleChannelsJob(entryTask *Task) {
	for {
		select {
		case du := <-lt.UserStatusChan:
			lt.handleUserStatus(du)
		case tr := <-lt.TaskRunChan:
			lt.handleTaskRun(tr)
		case s := <-lt.StatusChan:
			lt.handleStatus(s, entryTask)
		}
	}
}

func (lt *LoadTest) Run(entryTask *Task) {
	lt.Log.Println("Starting Load Testing Tool")

	if lt.Config.SpawnOnStartup {
		lt.SetStatus(StatusSpawning)
	}

	go lt.handleChannelsJob(entryTask)
	go lt.runAPIJob()
	go lt.cleanRPSJob()

	// Run forever
	c := make(chan struct{})
	<-c
}

func NewLoadTest(config Config) *LoadTest {
	return &LoadTest{
		Config:         config,
		Status:         StatusStopped,
		Users:          make(map[int64]User, config.NumUsers),
		StatusChan:     make(chan StatusType),
		UserStatusChan: make(chan User, config.NumUsers),
		Stats: Statistics{
			Tasks:           make(map[string]*TaskStats),
			RPSMap:          make(map[int64]int64),
			CurrentRPS:      0,
			AverageDuration: 0,
		},
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
