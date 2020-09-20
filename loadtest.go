package ltt

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type loadtestContextKeyType int

var loadtestContextKey loadtestContextKeyType

type Config struct {
	// Host to bind the REST API to. default all (empty string).
	APIHost string `json:"api_host"`
	// Port to bind the REST API to, default 4141
	APIPort  int `json:"api_port"`
	NumUsers int `json:"num_users"`
	// How many users to spawn each second
	NumSpawnPerSecond int `json:"num_spawn_per_second"`
	// Default 10 seconds
	RequestTimeout int `json:"request_timeout"`
	// Custom user type to override the DefaultUser
	UserType User `json:"-"`
	// Min sleep time between tasks in seconds
	MinSleepTime int `json:"min_sleep_time"`
	// Max sleep time between tasks in seconds
	MaxSleepTime int `json:"max_sleep_time"`
	// Verbose logging
	Verbose bool `json:"verbose"`
	// If we should start spawning users on startup
	SpawnOnStartup bool `json:"spawn_on_startup"`
	// Logging params
	LogOutput io.Writer `json:"-"`
	LogPrefix string    `json:"log_prefix"`
	LogFlags  int       `json:"log_flags"`
}

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

type TaskStats struct {
	sync.Mutex
	Name            string           `json:"name"`
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
	percentiles := []float64{0.5, 0.75, 0.85, 0.95, 0.99}

	if ts.TotalRuns < MinRunsToCalculate {
		// set to zero to make output more consistent
		for _, p := range percentiles {
			ts.Percentiles[int(p*100)] = 0
		}

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

func NewTaskStat(name string) *TaskStats {
	return &TaskStats{
		Name:        name,
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
	RunningUsers  int       `json:"num_users"`
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
	Config Config     `json:"config"`
	Status StatusType `json:"status"`
	// Map of created users
	UserMap     map[int64]User  `json:"-"`
	UserMapLock sync.Mutex      `json:"-"`
	StatusChan  chan StatusType `json:"-"`
	TaskRunChan chan *TaskRun   `json:"-"`
	Stats       *Statistics     `json:"stats"`
	Log         *log.Logger     `json:"-"`
	// Target number of user to spawn
	TargetUserNum int `json:"target_user_num"`
}

func NewConfigFromFlags() Config {
	conf := Config{}

	flag.IntVar(&conf.NumUsers, "num-users", 5, "Number of users to spawn")
	flag.IntVar(&conf.RequestTimeout, "request-timeout", 5, "Request timeout in seconds (Default 5)")
	flag.IntVar(&conf.MinSleepTime, "min-sleep-time", 1, "Minimum sleep time between a user's tasks in seconds (Default 1)")
	flag.IntVar(&conf.MaxSleepTime, "max-sleep-time", 10, "Maximum sleep time between a user's tasks in seconds (Default 10)")
	flag.IntVar(&conf.NumSpawnPerSecond, "num-spawn-per-sec", 1, "Number of user to spawn per second (Default 1)")
	flag.StringVar(&conf.APIHost, "api-host", "", "REST API port to bind to. (Default all, empty string)")
	flag.StringVar(&conf.LogPrefix, "log-prefix", "", "Logging prefix (Default empty string)")
	flag.IntVar(&conf.APIPort, "api-port", 4141, "REST API port to bind to. (Default 4141)")
	flag.BoolVar(&conf.Verbose, "verbose", false, "Verbose logging (default false)")
	flag.BoolVar(&conf.SpawnOnStartup, "spawn-on-startup", false, "If true, spawning will begin on startup (Default false)")
	flag.Parse()

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
				lt.SetStatus(StatusStopping)
			}
			diff = numUsers - lt.TargetUserNum
			lt.stopUsers(diff)
		} else if numUsers < lt.TargetUserNum {
			lt.SetStatus(StatusSpawning)
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
			lt.Log.Printf("pre-spawn sleep time: %d for user %d\n", sleepTime, u.ID())
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

func (lt *LoadTest) handleStatus(status StatusType) {
	lt.Status = status
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

func (lt *LoadTest) handleChannelsJob() {
	for {
		select {
		case tr := <-lt.TaskRunChan:
			lt.handleTaskRun(tr)
		case s := <-lt.StatusChan:
			lt.handleStatus(s)
		}
	}
}

func (lt *LoadTest) Run(entryTask *Task) {
	lt.Log.Println("Starting Load Testing Tool")

	if lt.Config.SpawnOnStartup {
		lt.TargetUserNum = lt.Config.NumUsers
	}

	go lt.handleChannelsJob()
	go lt.runAPIJob()
	go lt.cleanRPSJob()
	go lt.usersJob(entryTask)

	// Run forever
	c := make(chan struct{})
	<-c
}

func NewStatistics() *Statistics {
	return &Statistics{
		Tasks:           make(map[string]*TaskStats),
		RPSMap:          make(map[int64]int64),
		CurrentRPS:      0,
		AverageDuration: 0,
	}
}

func NewLoadTest(config Config) *LoadTest {
	return &LoadTest{
		Config:      config,
		Status:      StatusStopped,
		UserMap:     make(map[int64]User, config.NumUsers),
		StatusChan:  make(chan StatusType),
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
