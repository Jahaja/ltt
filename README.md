# Load Testing Tool

Largely based on the Python project Locust.

Sets up an HTTP API that provides the load test data in JSON.

# Usage
```
Usage of ltt:
  -api-host string
        REST API port to bind to.
  -api-port int
        REST API port to bind to. (default 4141)
  -log-prefix string
        Logging prefix
  -max-sleep-time int
        Maximum sleep time between a user's tasks in seconds (default 10)
  -min-sleep-time int
        Minimum sleep time between a user's tasks in seconds (default 1)
  -num-spawn-per-sec int
        Number of user to spawn per second (default 1)
  -num-users int
        Number of users to spawn (default 5)
  -request-timeout int
        Request timeout in seconds (default 5)
  -spawn-on-startup
        If true, spawning will begin on startup
  -verbose
        Verbose logging
```

## User-Interfaces

Terminal-UI
https://github.com/Jahaja/ltt-tui
