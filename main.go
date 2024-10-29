package main

import (
	"erebrusvps/docker"
	"fmt"
	"log"
	"net/http"
)

//lint:ignore U1000 logHandler is used to wrap HTTP handlers
func logHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("\n[API] %s %s\n", r.Method, r.URL.Path)
		handler(w, r)
	}
}

// Create a named handler function instead of an anonymous one
func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, World!")
}

func main() {
	// Initialize Docker setup
	dockerSetup := docker.NewDockerSetup()

	// Install Docker if not already installed
	err := dockerSetup.ExecuteCommand("sudo DEBIAN_FRONTEND=noninteractive apt-get -y update")
	if err != nil {
		log.Fatalf("Docker setup failed: %v", err)
	}

	// Install Nginx
	if err := dockerSetup.ExecuteCommand("sudo DEBIAN_FRONTEND=noninteractive apt-get install -y nginx"); err != nil {
		log.Fatalf("Nginx installation failed: %v", err)
	}

	// Add route handlers
	http.HandleFunc("/", logHandler(rootHandler))

	// Start the HTTP server
	fmt.Println("[SERVER] Starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
