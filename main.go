package main

import (
	"encoding/json"
	"erebrusvps/docker"
	"fmt"
	"log"
	"net/http"
)

// Add this handler wrapper for logging
func logHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("\n[API] %s %s\n", r.Method, r.URL.Path)
		handler(w, r)
	}
}

func main() {
	// Initialize Docker setup
	dockerSetup := docker.NewDockerSetup()

	// Install Docker if not already installed
	if err := dockerSetup.Install(); err != nil {
		log.Fatalf("Docker setup failed: %v", err)
	}
	// Install Nginx
	if err := dockerSetup.ExecuteCommand("sudo apt-get install -y nginx"); err != nil {
		log.Fatalf("Nginx installation failed: %v", err)
	}

	// API endpoint for deployments
	http.HandleFunc("/deploy", logHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fmt.Printf("[API] Method not allowed: %s\n", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var deployment docker.Deployment
		if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
			fmt.Printf("[API] Bad request: %v\n", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fmt.Printf("[API] Deploying project: %s\n", deployment.ProjectName)
		result, err := dockerSetup.DeployProject(deployment)
		if err != nil {
			fmt.Printf("[API] Deployment failed: %v\n", err)
			json.NewEncoder(w).Encode(docker.DeploymentResult{
				Status: "error",
				Error:  err.Error(),
			})
			return
		}

		fmt.Printf("[API] Deployment successful: %+v\n", result)
		json.NewEncoder(w).Encode(result)
	}))

	fmt.Println("[SERVER] Starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
