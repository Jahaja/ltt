# Load Testing Tool

Largely based on the Python project Locust.

Sets up an HTTP API that provides the load test data in JSON.

# Usage

### Example
```go
func main() {
	conf := ltt.NewConfigFromFlags()
	lt := ltt.NewLoadTest(conf)
  
        // Set up auth as an entry task for each simulated user
	entryTask := ltt.NewEntryTask("auth", func(ctx context.Context) error {
		user := ltt.UserFromContext(ctx)
		client := ltt.NewHTTPClient(ctx, "http://localhost:4000")

		ctx = ltt.NewHTTPClientContext(ctx, client)
		user.SetContext(ctx)

		data, _ := json.Marshal(map[string]string{
			"username": "",
			"password": "",
		})

		resp, err := client.Post("/v1/auth", data)
		if err != nil {
			return err
		}

		obj := make(map[string]string)
		err = resp.JSON(&obj)
		if err != nil {
			return err
		}

		client.Headers.Set("Authorization", "Bearer "+obj["token"])
		return nil
	}, ltt.TaskOptions{})
  
        // Add a profile section and a few sub tasks.
        // The next subtask to run is chosen by random according to their weight after
        // each task run + sleep (which simulates user usage/reading time)
	entryTask.AddSection("profile", func(t *ltt.Task) {
		t.AddSubTask("view", func(ctx context.Context) error {
			client := ltt.HTTPClientFromContext(ctx)
			_, err := client.Get("/v1/me")
			if err != nil {
				return err
			}
			return nil
		}, ltt.TaskOptions{Weight: 10})

		t.AddSubTask("edit", func(ctx context.Context) error {
			client := ltt.HTTPClientFromContext(ctx)
			_, err := client.Patch("/profile", nil)
			if err != nil {
				return err
			}
			return nil
		}, ltt.TaskOptions{Weight: 1})

	}, ltt.TaskOptions{Weight: 1})

	lt.Run(entryTask)
}
```

### CLI Options
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
