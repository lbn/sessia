package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	consul "github.com/hashicorp/consul/api"
)

var (
	consulClient *consul.Client
)

type Query struct {
	ID string `json:"id"`
}

func main() {
	consulConfig := consul.DefaultConfig()
	if consulAddr := os.Getenv("CONSUL_ADDR"); consulAddr != "" {
		consulConfig.Address = consulAddr
	}
	var err error
	consulClient, err = consul.NewClient(consulConfig)
	if err != nil {
		log.Fatal(err)
	}
	startRedisRefresher(1 * time.Second)

	router := mux.NewRouter()
	router.HandleFunc("/save", func(w http.ResponseWriter, req *http.Request) {
		if req.Body == nil {
			log.Fatal("nil body")
		}
		defer req.Body.Close()
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			log.Fatal(err)
		}
		// TODO: error handling
		id := saveEntry(body)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{
			"id": id,
		})

	}).Methods("POST")

	router.HandleFunc("/query", func(w http.ResponseWriter, req *http.Request) {
		if req.Body == nil {
			log.Fatal("nil body")
		}
		defer req.Body.Close()

		var query Query
		err := json.NewDecoder(req.Body).Decode(&query)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		data, err := getEntry(query.ID)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{
			"data": string(data),
		})

	}).Methods("POST")

	log.Fatal(http.ListenAndServe(":8000", router))
}
