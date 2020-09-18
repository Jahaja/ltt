package ltt

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func RunAPIServer(lt *LoadTest) error {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		lt.Stats.Lock()
		lt.Stats.Calculate()
		data, err := json.Marshal(lt.Stats)
		lt.Stats.Unlock()

		if err != nil {
			lt.Log.Printf("error marshalling stats: %s\n", err.Error())
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write(data)
	})

	http.HandleFunc("/start", func(writer http.ResponseWriter, request *http.Request) {
		lt.SetStatus(StatusSpawning)
		writer.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/stop", func(writer http.ResponseWriter, request *http.Request) {
		lt.SetStatus(StatusStopping)
		writer.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/reset", func(writer http.ResponseWriter, request *http.Request) {
		lt.Stats.Lock()
		lt.Stats.Reset()
		lt.Stats.Unlock()
		writer.WriteHeader(http.StatusOK)
	})

	lt.Log.Printf("Starting REST API on %s:%d", lt.Config.APIHost, lt.Config.APIPort)
	err := http.ListenAndServe(fmt.Sprintf("%s:%d", lt.Config.APIHost, lt.Config.APIPort), nil)
	if err != nil {
		return err
	}

	return nil
}