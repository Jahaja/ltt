package loadtest

import (
	"context"
	"flag"
	"log"
	"reflect"
	"sync"
	"time"
)

type loadtestContextKeyType int

var loadtestContextKey loadtestContextKeyType

type Config struct {
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

type LoadTest struct {
	Config Config
	Status Status

	DefaultUserSpawn func(User)
	DefaultUserSleep func(User)
	SpawnedUsers     []User
	UserSpawnChan    chan User
}

func NewConfigFromFlags() Config {
	var req_timeout int
	conf := Config{}

	flag.IntVar(&conf.NumUsers, "num-users", 2, "Number of users to spawn")
	flag.IntVar(&req_timeout, "request-timeout", 5000, "Request timeout in ms (Default 5000)")
	flag.IntVar(&conf.MinSleepTime, "min-sleep-time", 1, "Minimum sleep time between a user's tasks in seconds (Default 1)")
	flag.IntVar(&conf.MaxSleepTime, "max-sleep-time", 10, "Maximum sleep time between a user's tasks in seconds (Default 10)")
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
			du := <-lt.UserSpawnChan
			lt.SpawnedUsers = append(lt.SpawnedUsers, du)
			if len(lt.SpawnedUsers) == lt.Config.NumUsers {
				lt.Status = StatusRunning
				log.Printf("all %d users have been spawned, status changed to running\n", lt.Config.NumUsers)
			}
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
