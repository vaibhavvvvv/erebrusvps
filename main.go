package main

import (
	"erebrusvps/docker"
	"log"
)

func main() {
	dockerSetup := docker.NewDockerSetup()
	if err := dockerSetup.Install(); err != nil {
		log.Fatalf("Docker setup failed: %v", err)
	}
}
