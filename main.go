package main

import (
	"log"
	"os"

	docker "erebrusvps/docker"
)

func main() {
	// Get sudo password from environment variable for security
	sudoPassword := os.Getenv("SUDO_PASSWORD")
	if sudoPassword == "" {
		log.Fatal("Please set SUDO_PASSWORD environment variable")
	}

	dockerSetup := docker.NewDockerSetup(sudoPassword)
	if err := dockerSetup.Install(); err != nil {
		log.Fatalf("Docker setup failed: %v", err)
	}
}
