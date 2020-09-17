package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Jahaja/ltt"
)

func help() {
	fmt.Println("Load Testing Tool")
	fmt.Println("")
	fmt.Println("Options")
	flag.PrintDefaults()
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime)
	log.SetPrefix("")
}

func main() {
	conf := ltt.NewConfigFromFlags()
	lt := ltt.New(conf)

	lt.DefaultUserSpawn = func(user ltt.User) {
		ctx := user.Context()
		client := ltt.NewHTTPClient(ctx, "http://localhost:4000")

		ctx = ltt.NewHTTPClientContext(ctx, client)
		user.SetContext(ctx)

		data, _ := json.Marshal(map[string]string{
			"username": "",
			"password": "",
		})

		resp, err := client.Post("/v1/auth", data)
		if err != nil {
			log.Printf("failed to login: %s\n", err.Error())
			return
		}

		obj := make(map[string]string)
		err = resp.JSON(&obj)

		if err == nil {
			client.Headers.Set("Authorization", "Bearer "+obj["token"])
		}
	}

	entry_task := ltt.NewEntryTask("MyProject", ltt.TaskOptions{})

	entry_task.AddSection("profile", func(t *ltt.Task) {
		t.AddSubTask("view", func(ctx context.Context) error {
			client := ltt.HTTPFromContext(ctx)
			_, err := client.Get("/v1/me")
			if err != nil {
				return err
			}
			return nil
		}, ltt.TaskOptions{Weight: 10})

		t.AddSubTask("edit", func(ctx context.Context) error {
			client := ltt.HTTPFromContext(ctx)
			_, err := client.Patch("/profile", nil)
			if err != nil {
				return err
			}
			return nil
		}, ltt.TaskOptions{Weight: 1})

	}, ltt.TaskOptions{Weight: 1, SelectionStrategy: ltt.TaskSelectionStrategyRandom})

	lt.Run(entry_task)
}
