package main

import (
	"context"
	"encoding/json"
	"github.com/Jahaja/ltt"
)

func main() {
	conf := ltt.NewConfigFromFlags()
	lt := ltt.NewLoadTest(conf)

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
