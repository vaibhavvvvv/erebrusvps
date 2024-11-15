package main

import (
	"encoding/json"
	"erebrusvps/docker"
	"erebrusvps/websocket"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// Add certificate generation function
func generateSSLCertificates(dockerSetup *docker.DockerSetup) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	certDir := filepath.Join(homeDir, "certs")
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("failed to create certs directory: %v", err)
	}

	// Generate self-signed certificate
	cmd := fmt.Sprintf(`openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
		-keyout %s/server.key \
		-out %s/server.crt \
		-subj "/C=US/ST=State/L=City/O=Organization/CN=localhost"`,
		certDir, certDir)

	if err := dockerSetup.ExecuteCommand(cmd); err != nil {
		return fmt.Errorf("failed to generate SSL certificates: %v", err)
	}

	// Move certificates to nginx directory
	if err := dockerSetup.ExecuteCommand(fmt.Sprintf("sudo cp %s/server.crt /etc/nginx/ssl/", certDir)); err != nil {
		return err
	}
	if err := dockerSetup.ExecuteCommand(fmt.Sprintf("sudo cp %s/server.key /etc/nginx/ssl/", certDir)); err != nil {
		return err
	}

	return nil
}

func main() {
	// Initialize Docker setup
	dockerSetup := docker.NewDockerSetup()

	// Install required packages
	err := dockerSetup.ExecuteCommand("sudo DEBIAN_FRONTEND=noninteractive apt-get -y update")
	if err != nil {
		log.Fatalf("Update failed: %v", err)
	}

	// Install Nginx and OpenSSL
	if err := dockerSetup.ExecuteCommand("sudo DEBIAN_FRONTEND=noninteractive apt-get install -y nginx openssl"); err != nil {
		log.Fatalf("Nginx/OpenSSL installation failed: %v", err)
	}

	// Create SSL directory for Nginx
	if err := dockerSetup.ExecuteCommand("sudo mkdir -p /etc/nginx/ssl"); err != nil {
		log.Fatalf("Failed to create SSL directory: %v", err)
	}

	// Generate SSL certificates
	if err := generateSSLCertificates(dockerSetup); err != nil {
		log.Fatalf("Failed to generate SSL certificates: %v", err)
	}

	// Get home directory for certificates
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}
	certDir := filepath.Join(homeDir, "certs")

	// Add CORS and handlers
	http.HandleFunc("/deploy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		deploymentHandler(w, r)
	})

	// Add WebSocket handler
	http.HandleFunc("/ws", websocket.Logger.HandleWebSocket)

	// Start HTTPS server
	fmt.Println("[SERVER] Starting HTTPS server on :8443")
	go func() {
		if err := http.ListenAndServeTLS(":8443",
			filepath.Join(certDir, "server.crt"),
			filepath.Join(certDir, "server.key"),
			nil); err != nil {
			log.Fatal(err)
		}
	}()

	// Redirect HTTP to HTTPS
	fmt.Println("[SERVER] Starting HTTP redirect server on :8080")
	if err := http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.Host+r.URL.String(), http.StatusMovedPermanently)
	})); err != nil {
		log.Fatal(err)
	}
}
