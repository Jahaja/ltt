package ltt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func RunAPIServer(lt *LoadTest) error {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if lt.Config.Verbose {
			lt.Log.Println("http: / request")
		}

		lt.Stats.Lock()
		lt.Stats.Calculate()
		data, err := json.Marshal(lt)
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

	http.HandleFunc("/set-num-users", func(writer http.ResponseWriter, request *http.Request) {
		numUsers, _ := strconv.Atoi(request.URL.Query().Get("num-users"))
		lt.Log.Printf("http: /set-num-users request, num-users: %d\n", numUsers)
		lt.TargetUserNum = numUsers
		writer.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/reset", func(writer http.ResponseWriter, request *http.Request) {
		lt.Log.Println("http: /reset request")
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
