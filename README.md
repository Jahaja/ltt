# Load Testing Tool

Largely based on the Python project Locust.

Sets up an HTTP API that provides the load test data in JSON.

# Usage
```
Usage of ltt:
  -api-host string
        REST API port to bind to. (Default all, empty string)
  -api-port int
        REST API port to bind to. (Default 4141) (default 4141)
  -log-prefix string
        Logging prefix (Default empty string)
  -max-sleep-time int
        Maximum sleep time between a user's tasks in seconds (Default 10) (default 10)
  -min-sleep-time int
        Minimum sleep time between a user's tasks in seconds (Default 1) (default 1)
  -num-spawn-per-sec int
        Number of user to spawn per second (Default 1) (default 1)
  -num-users int
        Number of users to spawn (default 5)
  -request-timeout int
        Request timeout in seconds (Default 5) (default 5)
  -spawn-on-startup
        If true, spawning will begin on startup (Default false)
  -verbose
        Verbose logging (default false)
```

## User-Interfaces

Terminal-UI
https://github.com/Jahaja/ltt-tui
