package main

import (
	"encoding/json"
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

// Simplified request structure matching docker.Deployment
type DeploymentRequest struct {
	GitURL  string            `json:"git_url"`
	EnvVars map[string]string `json:"env_vars,omitempty"`
}

// Add deployment handler
func deploymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var deployment docker.Deployment
	if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
		http.Error(w, "Error parsing JSON", http.StatusBadRequest)
		return
	}

	dockerSetup := docker.NewDockerSetup()
	result, err := dockerSetup.DeployProject(deployment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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
	http.HandleFunc("/deploy", logHandler(deploymentHandler))

	// Start the HTTP server
	fmt.Println("[SERVER] Starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
