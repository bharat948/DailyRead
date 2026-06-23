package main

import (
	"log"
	"dailyread/cmd/dailyread/cli"
	"github.com/joho/godotenv"
)

func main() {
	// Attempt to load .env file if it exists
	_ = godotenv.Load()

	if err := cli.Execute(); err != nil {
		log.Fatalf("Error executing command: %v\n", err)
	}
}
