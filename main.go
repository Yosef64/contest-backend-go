package main

import (
	"victor-contest-go/internal/handler/http"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	server := http.NewServer()
	r := server.NewRouter()
	
	r.Run(":8080") 
} 