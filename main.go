package main

import (
	"encoding/json"
	"erebrusvps/docker"
	"log"
	"net/http"
)

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
	http.HandleFunc("/deploy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var deployment docker.Deployment
		if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		result, err := dockerSetup.DeployProject(deployment)
		if err != nil {
			json.NewEncoder(w).Encode(docker.DeploymentResult{
				Status: "error",
				Error:  err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(result)
	})

	// Start the server
	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
