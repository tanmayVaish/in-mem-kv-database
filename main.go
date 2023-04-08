package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type keyValueStore struct {
	mu     sync.RWMutex
	store  map[string]*entry
	expiry map[string]time.Time
}

type entry struct {
	value    string
	expiry   time.Time
	exist    bool
	condType string
}

func main() {
	kv := &keyValueStore{
		store:  make(map[string]*entry),
		expiry: make(map[string]time.Time),
	}
	http.HandleFunc("/set", kv.setHandler)
	http.HandleFunc("/get", kv.getHandler)
	http.HandleFunc("/qpush", kv.qpushHandler)

	fmt.Println("Starting server on port 8080")
	http.ListenAndServe(":8080", nil)
}

func (kv *keyValueStore) setHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Expiry   int    `json:"expiry,omitempty"`
		CondType string `json:"condition,omitempty"`
	}

	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "Invalid request data", http.StatusBadRequest)
		return
	}

	if data.Key == "" || data.Value == "" {
		http.Error(w, "Key and value are required", http.StatusBadRequest)
		return
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	_, ok := kv.store[data.Key]

	if ok && data.CondType == "NX" {
		http.Error(w, "Key already exists", http.StatusConflict)
		return
	} else if !ok && data.CondType == "XX" {
		http.Error(w, "Key does not exist", http.StatusNotFound)
		return
	}

	if data.Expiry != 0 {
		if _, err := time.ParseDuration(strconv.Itoa(data.Expiry) + "s"); err != nil {
			http.Error(w, "Invalid expiry value", http.StatusBadRequest)
			return
		}
		expiry := time.Now().Add(time.Duration(data.Expiry) * time.Second)
		kv.expiry[data.Key] = expiry
	}

	kv.store[data.Key] = &entry{value: data.Value, exist: true, expiry: kv.expiry[data.Key], condType: data.CondType}

	w.WriteHeader(http.StatusCreated)
}

func (kv *keyValueStore) getHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")

	if key == "" {
		http.Error(w, "Key is required", http.StatusBadRequest)
		return
	}

	kv.mu.RLock()
	defer kv.mu.RUnlock()

	e, ok := kv.store[key]

	if !ok || (e.expiry != time.Time{} && e.expiry.Before(time.Now())) {
		http.Error(w, "Key not found or expired", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"value": e.value})
}

func (kv *keyValueStore) qpushHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Key    string   `json:"key"`
		Values []string `json:"values"`
	}

	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "Invalid request data", http.StatusBadRequest)
		return
	}

	if data.Key == "" || len(data.Values) == 0 {
		http.Error(w, "Key and values are required", http.StatusBadRequest)
		return
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	if _, ok := kv.store[data.Key]; !ok {
		kv.store[data.Key] = &entry{value: "", exist: true}
	}

	queue := make([]string, 0)
	if kv.store[data.Key].value != "" {
		err = json.Unmarshal([]byte(kv.store[data.Key].value), &queue)
		if err != nil {
			http.Error(w, "Unable to unmarshal queue data", http.StatusInternalServerError)
			return
		}
	}

	queue = append(queue, data.Values...)
	queueData, err := json.Marshal(queue)
	if err != nil {
		http.Error(w, "Unable to marshal queue data", http.StatusInternalServerError)
		return
	}

	kv.store[data.Key].value = string(queueData)

	w.WriteHeader(http.StatusCreated)
}
