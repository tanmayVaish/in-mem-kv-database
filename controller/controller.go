package controller

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type keyValueStore struct {
	mu    sync.RWMutex
	store map[string]*entry
}

type entry struct {
	value  string
	expiry time.Time
	exist  bool
}

func Controller(c *gin.Context) {

	kv := &keyValueStore{
		store: make(map[string]*entry),
	}

	command_type := c.MustGet("command-type").(string)
	command := c.MustGet("command").(string)

	words := strings.Split(command, " ")

	// words[0] = command_type
	// words[1] = key
	// words[2] = value
	// words[3] = duration
	// words[4] = condition (NX or XX)

	switch command_type {
	case "SET":
		kv.mu.Lock()

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
				// TODO: handle error
				duration, _ = time.ParseDuration(words[4] + "s")
			}

			// if EX is not there, check for PX at words[3]
			if len(words) > 5 {
				if words[5] == "NX" {
					condition = words[5]
				} else if words[5] == "XX" {
					condition = words[5]
				}
			}
		}

		if condition == "NX" {
			if _, ok := kv.store[key]; !ok {
				expiry := time.Now().Add(duration)
				kv.store[key] = &entry{value, expiry, true}
			}
		} else if condition == "XX" {
			if _, ok := kv.store[key]; ok {
				expiry := time.Now().Add(duration)
				kv.store[key] = &entry{value, expiry, true}
			}
		} else {
			expiry := time.Now().Add(duration)
			kv.store[key] = &entry{value, expiry, true}
		}

		fmt.Println("key", key, "value", value, "condition", condition, "duration", duration)

		c.JSON(200, gin.H{"status": "OK"})

		defer kv.mu.Unlock()

	case "GET":
		// kv.mu.RLock()
		// defer kv.mu.RUnlock()

		key := words[1]
		if val, ok := kv.store[key]; ok {
			if val.expiry.IsZero() || val.expiry.After(time.Now()) {
				c.JSON(200, gin.H{"value": val.value})
			} else {
				delete(kv.store, key)
				c.JSON(404, gin.H{"error": "key not found"})
			}
		} else {
			c.JSON(404, gin.H{"error": "key not found"})
		}

	case "QPUSH":
		// kv.mu.Lock()
		// defer kv.mu.Unlock()

		key := words[1]
		value := words[2]

		if _, ok := kv.store[key]; !ok {
			kv.store[key] = &entry{"", time.Time{}, true}
		}

		kv.store[key].value = value + kv.store[key].value
		c.JSON(200, gin.H{"length": len(kv.store[key].value)})
	default:
		c.JSON(400, gin.H{"error": "Invalid command"})
	}

}
