package main

import (
	"flag"
	"log"

	"fyp-data/server"
)

func main() {
	configPath := flag.String("config", "config/api.example.json", "path to API JSON config")
	flag.Parse()

	if err := server.Run(*configPath); err != nil {
		log.Fatal(err)
	}
}
