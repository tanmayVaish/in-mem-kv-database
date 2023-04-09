package main

import (
	"fmt"
	"in-mem-kv-database/route"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// extract PORT from environment variable
	port := os.Getenv("PORT")

	route.CommandRoute(r)

	fmt.Println("Starting server on port " + port)
	r.Run(":" + port)
}
