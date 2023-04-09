package main

import (
	"fmt"
	"in-mem-kv-database/route"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	route.CommandRoute(r)

	fmt.Println("Starting server on port 8080")
	r.Run(":8080")
}
