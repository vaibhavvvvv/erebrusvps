package main

import (
	"encoding/json"
	"erebrusvps/docker"
	"erebrusvps/websocket"
	"fmt"
	"log"
	"net/http"
	"strings"
)

//lint:ignore U1000 logHandler is used to wrap HTTP handlers
func logHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("\n[API] %s %s\n", r.Method, r.URL.Path)
		handler(w, r)
	}
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

	// Validate required fields
	if deployment.GitURL == "" {
		http.Error(w, "git_url is required", http.StatusBadRequest)
		return
	}

	// Set default port if not provided
	if deployment.Port == "" {
		deployment.Port = "3000" // or generate a random available port
	}

	// Set default project name if not provided
	if deployment.ProjectName == "" {
		// Extract project name from git URL
		parts := strings.Split(deployment.GitURL, "/")
		deployment.ProjectName = strings.TrimSuffix(parts[len(parts)-1], ".git")
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
	http.HandleFunc("/deploy", logHandler(deploymentHandler))

	// Add WebSocket handler
	http.HandleFunc("/ws", websocket.Logger.HandleWebSocket)

	// Start the HTTP server
	fmt.Println("[SERVER] Starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
