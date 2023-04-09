package route

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

func CommandRoute(r *gin.Engine) {

	kv := &keyValueStore{
		store:  make(map[string]*entry),
		expiry: make(map[string]time.Time),
	}

	fmt.Print("Setting up routes...")

	r.POST("/set", kv.setHandler)
	r.GET("/get", kv.getHandler)
	r.POST("/qpush", kv.qpushHandler)

}

func (kv *keyValueStore) setHandler(c *gin.Context) {

	// request body is in JSON format
	// {
	// 	"command" : "SET key value EX 10 NX"
	// }

	var requestBody map[string]string

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	words := strings.Split(requestBody["command"], " ")

	// words[0] = "SET"
	// words[1] = key
	// words[2] = value
	// words[3] = EX
	// words[4] = duration
	// words[5] = NX or XX

	if len(words) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid command"})
		return
	}

	key := words[1]
	value := words[2]
	condition := ""
	duration := time.Duration(0)

	if len(words) > 3 {
		if words[3] == "NX" {
			condition = words[3]
		} else if words[3] == "XX" {
			condition = words[3]
		} else if words[3] == "EX" {
			duration, _ = time.ParseDuration(words[4] + "s")
		}

		// if EX is there, check for NX or XX at words[5]
		if len(words) > 5 {
			if words[5] == "NX" {
				condition = words[5]
			} else if words[5] == "XX" {
				condition = words[5]
			}
		}
	}

	var data struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Expiry   int    `json:"expiry,omitempty"`
		CondType string `json:"condition,omitempty"`
	}

	data.Key = key
	data.Value = value
	data.Expiry = int(duration.Seconds())
	data.CondType = condition

	fmt.Println(data)

	if data.Key == "" || data.Value == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Key and value are required"})
		return
	}

	kv.mu.Lock()
	defer kv.mu.Unlock()

	_, ok := kv.store[data.Key]

	if ok && data.CondType == "NX" {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Key already exists"})
		return
	} else if !ok && data.CondType == "XX" {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Key does not exist"})
		return
	}

	if data.Expiry != 0 {
		if _, err := time.ParseDuration(strconv.Itoa(data.Expiry) + "s"); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry value"})
			return
		}
		expiry := time.Now().Add(time.Duration(data.Expiry) * time.Second)
		kv.expiry[data.Key] = expiry
	}

	kv.store[data.Key] = &entry{value: data.Value, exist: true, expiry: kv.expiry[data.Key], condType: data.CondType}

	c.Status(http.StatusCreated)
}

func (kv *keyValueStore) getHandler(c *gin.Context) {

	// request body is in JSON format
	// {
	// 	"command" : "GET key"
	// }

	var requestBody map[string]string

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	words := strings.Split(requestBody["command"], " ")

	// words[0] = "GET"
	// words[1] = key

	if len(words) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid command"})
		return
	}

	key := words[1]

	if key == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Key is required"})
		return
	}

	kv.mu.RLock()
	defer kv.mu.RUnlock()

	e, ok := kv.store[key]

	if !ok || (e.expiry != time.Time{} && e.expiry.Before(time.Now())) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Key not found or expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"value": e.value})
}

func (kv *keyValueStore) qpushHandler(c *gin.Context) {

	// request body is in JSON format
	// {
	//  "command" : "QPUSH key value1 value2 value3 valuen"
	// }

	var requestBody map[string]string

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	words := strings.Split(requestBody["command"], " ")

	// words[0] = "QPUSH"
	// words[1] = key
	// words[2] = value1
	// words[3] = value2
	// words[4] = value3
	// words[n] = valuen

	if len(words) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid command"})
		return
	}

	key := words[1]
	values := words[2:]

	if key == "" || len(values) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Key and values are required"})
		return
	}

	var data struct {
		Key    string   `json:"key"`
		Values []string `json:"values"`
	}

	data.Key = key
	data.Values = values

	err := c.BindJSON(&data)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	if data.Key == "" || len(data.Values) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Key and values are required"})
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
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Unable to unmarshal queue data"})
			return
		}
	}

	queue = append(queue, data.Values...)
	queueData, err := json.Marshal(queue)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Unable to marshal queue data"})
		return
	}

	kv.store[data.Key].value = string(queueData)

	c.Status(http.StatusCreated)
}
