package main

import (
	"victor-contest-go/internal/handler/http"
)

func main() {
	server := http.NewServer()
	r := server.NewRouter()
	r.Run(":8080")
} 