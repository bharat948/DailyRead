package main

import (
	"log"
	"dailyread/cmd/dailyread/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Fatalf("Error executing command: %v\n", err)
	}
}
