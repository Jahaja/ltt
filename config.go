package ltt

import (
	"flag"
	"io"
	"os"
)

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

func NewConfigFromFlags() Config {
	conf := Config{}

	flag.IntVar(&conf.NumUsers, "num-users", 5, "Number of users to spawn")
	flag.IntVar(&conf.RequestTimeout, "request-timeout", 5, "Request timeout in seconds")
	flag.IntVar(&conf.MinSleepTime, "min-sleep-time", 1, "Minimum sleep time between a user's tasks in seconds")
	flag.IntVar(&conf.MaxSleepTime, "max-sleep-time", 10, "Maximum sleep time between a user's tasks in seconds")
	flag.IntVar(&conf.NumSpawnPerSecond, "num-spawn-per-sec", 1, "Number of user to spawn per second")
	flag.StringVar(&conf.APIHost, "api-host", "", "REST API port to bind to.")
	flag.StringVar(&conf.LogPrefix, "log-prefix", "", "Logging prefix")
	flag.IntVar(&conf.APIPort, "api-port", 4141, "REST API port to bind to.")
	flag.BoolVar(&conf.Verbose, "verbose", false, "Verbose logging")
	flag.BoolVar(&conf.SpawnOnStartup, "spawn-on-startup", false, "If true, spawning will begin on startup")
	flag.Parse()

	if conf.LogOutput == nil {
		conf.LogOutput = os.Stdout
	}

	return conf
}
