package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"ltt/clients"
	"ltt/loadtest"
	"os"
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
	conf := loadtest.NewConfigFromFlags()
	lt := loadtest.New(conf)

	lt.DefaultUserSpawn = func(user loadtest.User) {
		ctx := user.Context()
		client := clients.NewHTTPClient(ctx, "http://localhost:4000")

		ctx = clients.NewHTTPClientContext(ctx, client)
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

	entry_task := loadtest.NewEntryTask("MyProject", loadtest.TaskOptions{})

	entry_task.AddSection("profile", func(t *loadtest.Task) {
		t.AddTask("view", func(ctx context.Context) error {
			client := clients.HTTPFromContext(ctx)
			resp, err := client.Get("/v1/me")
			if err != nil {
				return err
			}
			log.Println("Response: ", string(resp.Body))
			return nil
		}, loadtest.TaskOptions{Weight: 10})

		t.AddTask("edit", func(ctx context.Context) error {
			client := clients.HTTPFromContext(ctx)
			resp, err := client.Patch("/profile", nil)
			if err != nil {
				return err
			}
			log.Println("Response: ", string(resp.Body))
			return nil
		}, loadtest.TaskOptions{Weight: 1})

	}, loadtest.TaskOptions{Weight: 1, SelectionStrategy: loadtest.TaskSelectionStrategyRandom})

	lt.Run(entry_task)
}
