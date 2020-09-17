package ltt

import (
	"context"
	"flag"
	"log"
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
}

type Status int

const (
	StatusStopped Status = iota
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
	NumTotal      int64 `json:"num_total"`
	NumSuccessful int64 `json:"num_successful"`
	NumFailed     int64 `json:"num_failed"`
	TotalDuration int64 `json:"total_duration"`
	// unix-timestamp -> count map to calculate a current RPS value
	RPSMap map[int64]int64 `json:"-"`

	Tasks           map[string]*TaskStats `json:"tasks"`
	CurrentRPS      float32               `json:"current_rps"`
	AverageDuration float32               `json:"average_duration"`
}

const RPSTimeWindow = 10

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
	Status Status

	StartTime        time.Time
	DefaultUserSpawn func(User)
	DefaultUserSleep func(User)
	SpawnedUsers     []User
	UserSpawnChan    chan User
	TaskRunChan      chan *TaskRun
	Stats            Statistics
}

func NewConfigFromFlags() Config {
	var req_timeout int
	conf := Config{}

	flag.IntVar(&conf.NumUsers, "num-users", 5, "Number of users to spawn")
	flag.IntVar(&req_timeout, "request-timeout", 5000, "Request timeout in ms (Default 5000)")
	flag.IntVar(&conf.MinSleepTime, "min-sleep-time", 1, "Minimum sleep time between a user's tasks in seconds (Default 1)")
	flag.IntVar(&conf.MaxSleepTime, "max-sleep-time", 10, "Maximum sleep time between a user's tasks in seconds (Default 10)")
	flag.StringVar(&conf.APIHost, "api-host", "", "REST API port to bind to. (Default all, empty string)")
	flag.IntVar(&conf.APIPort, "api-port", 4141, "REST API port to bind to. (Default 4141)")
	flag.Parse()

	conf.RequestTimeout = time.Millisecond * time.Duration(req_timeout)
	return conf
}

func (lt *LoadTest) Stop() {
	lt.Status = StatusStopping
}

func (lt *LoadTest) Run(entry_task *Task) {
	log.Println("Starting load test tool")

	lt.Status = StatusSpawning
	go func() {
		for {
			select {
			case du := <-lt.UserSpawnChan:
				lt.SpawnedUsers = append(lt.SpawnedUsers, du)
				if len(lt.SpawnedUsers) == lt.Config.NumUsers {
					lt.Status = StatusRunning
					log.Printf("all %d users have been spawned, status changed to running\n", lt.Config.NumUsers)
				}
			case tr := <-lt.TaskRunChan:
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
		}
	}()

	go func() {
		err := RunAPIServer(lt)
		if err != nil {
			log.Println("failed to start api server: %s", err.Error())
		}
	}()

	go func() {
		for {
			lt.Stats.Lock()
			lt.Stats.CleanRPSMap()
			lt.Stats.Unlock()
			time.Sleep(time.Second * 30)
		}
	}()

	wg := sync.WaitGroup{}
	for i := 0; i < lt.Config.NumUsers; i++ {
		// Create a new User instance
		var u User
		uv := reflect.ValueOf(lt.Config.UserType)
		if !uv.IsValid() {
			u = NewDefaultUser(entry_task)
		} else {
			ok := false
			u, ok = reflect.New(uv.Type()).Interface().(User)
			if !ok {
				log.Fatalf("failed to cast LoadTest.User to User\n")
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

		wg.Add(1)
		go func() {
			defer wg.Done()

			u.Spawn()
			lt.UserSpawnChan <- u
			for lt.Status != StatusStopping {
				u.Tick()
				u.Sleep()
			}
		}()
	}
	wg.Wait()
}

func New(config Config) *LoadTest {
	return &LoadTest{
		Config:        config,
		Status:        StatusStopped,
		UserSpawnChan: make(chan User, config.NumUsers),
		Stats: Statistics{
			Tasks:  make(map[string]*TaskStats),
			RPSMap: make(map[int64]int64),
		},
		TaskRunChan: make(chan *TaskRun, config.NumUsers),
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
