package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	// "encoding/json"
	"erebrusvps/websocket"
)

type Deployment struct {
	GitURL      string            `json:"git_url"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	Port        string            `json:"port"`
	ProjectName string            `json:"project_name"`
}

type DeploymentResult struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	Port   string `json:"port"`
	Error  string `json:"error,omitempty"`
}

type PortMapping struct {
	Port        string
	ProjectName string
	GitURL      string
}

var usedPorts = make(map[string]PortMapping) // key: port number, value: project details
var startingPort = 3000

func getNextAvailablePort() string {
	port := startingPort
	for {
		portStr := fmt.Sprintf("%d", port)
		// Check if port is used by our deployments and system
		if _, exists := usedPorts[portStr]; !exists && isPortAvailable(portStr) {
			return portStr
		}
		port++
	}
}

func (d *DockerSetup) DeployProject(deployment Deployment) (*DeploymentResult, error) {
	// Send logs through WebSocket
	sendLog := func(message string) {
		websocket.Logger.SendLog(message)
		fmt.Println(message) // Still print to console
	}

	sendLog(fmt.Sprintf("\n[DEPLOY] Starting deployment for project: %s", deployment.ProjectName))

	// Always get next available port if the requested port is in use
	if deployment.Port == "" || !isPortAvailable(deployment.Port) {
		newPort := getNextAvailablePort()
		sendLog(fmt.Sprintf("[DEPLOY] Port %s is occupied, assigning port %s for project %s",
			deployment.Port, newPort, deployment.ProjectName))
		deployment.Port = newPort
	}

	// Store the port mapping
	usedPorts[deployment.Port] = PortMapping{
		Port:        deployment.Port,
		ProjectName: deployment.ProjectName,
		GitURL:      deployment.GitURL,
	}

	// Use home directory instead of /opt
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %v", err)
	}

	// Create workspace directory
	workDir := filepath.Join(homeDir, "deployments", deployment.ProjectName)
	sendLog(fmt.Sprintf("[DEPLOY] Creating workspace directory: %s", workDir))
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %v", err)
	}

	// Clone repository
	sendLog(fmt.Sprintf("[DEPLOY] Cloning repository: %s", deployment.GitURL))
	if err := d.cloneRepository(deployment.GitURL, workDir); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %v", err)
	}

	// Create Dockerfile if it doesn't exist
	sendLog("[DEPLOY] Ensuring Dockerfile exists")
	if err := d.ensureDockerfile(workDir); err != nil {
		return nil, fmt.Errorf("failed to create Dockerfile: %v", err)
	}

	// Create docker-compose.yml
	sendLog("[DEPLOY] Creating docker-compose.yml")
	if err := d.createDockerCompose(workDir, deployment); err != nil {
		return nil, fmt.Errorf("failed to create docker-compose.yml: %v", err)
	}

	// Build and run the container
	sendLog("[DEPLOY] Building and running containers")
	if err := d.buildAndRun(workDir, deployment); err != nil {
		return nil, fmt.Errorf("failed to build and run: %v", err)
	}

	// Configure Nginx reverse proxy
	sendLog("[DEPLOY] Configuring Nginx reverse proxy")
	if err := d.configureNginx(deployment); err != nil {
		return nil, fmt.Errorf("failed to configure nginx: %v", err)
	}

	sendLog("[DEPLOY] Deployment completed successfully!")
	fmt.Println(&DeploymentResult{
		Status: "success",
		URL:    fmt.Sprintf("https://%s.localhost", deployment.ProjectName),
		Port:   deployment.Port,
	})

	return &DeploymentResult{
		Status: "success",
		URL:    fmt.Sprintf("https://%s.localhost", deployment.ProjectName),
		Port:   deployment.Port,
	}, nil
}

func (d *DockerSetup) cloneRepository(gitURL, workDir string) error {
	fmt.Printf("[GIT] Cloning repository from %s to %s\n", gitURL, workDir)

	// Check if directory exists
	if _, err := os.Stat(workDir); err == nil {
		fmt.Printf("[GIT] Directory exists, removing it first...\n")
		if err := os.RemoveAll(workDir); err != nil {
			return fmt.Errorf("failed to clean existing directory: %v", err)
		}
	}

	// Create fresh directory
	if err := os.MkdirAll(filepath.Dir(workDir), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %v", err)
	}

	cmd := exec.Command("git", "clone", gitURL, workDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}

	fmt.Printf("[GIT] Repository cloned successfully\n")
	return nil
}

func (d *DockerSetup) ensureDockerfile(workDir string) error {
	dockerfilePath := filepath.Join(workDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		// Create a default Dockerfile for React applications
		dockerfile := `FROM node:16-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build
EXPOSE 8080
RUN npm install -g serve
CMD ["serve", "-s", "build", "-l", "8080"]`
		return os.WriteFile(dockerfilePath, []byte(dockerfile), 0644)
	}
	return nil
}

func (d *DockerSetup) createDockerCompose(workDir string, deployment Deployment) error {
	template := `services:
  app:
    build: .
    ports:
      - "%s:%s"
    environment:
      PORT: "%s"
    restart: always
    networks:
      - deployment-network

networks:
  deployment-network:
    external: true`

	compose := fmt.Sprintf(template,
		deployment.Port,
		"8080", // internal port
		"8080", // environment variable PORT
	)

	return os.WriteFile(filepath.Join(workDir, "docker-compose.yml"), []byte(compose), 0644)
}

func (d *DockerSetup) buildAndRun(workDir string, deployment Deployment) error {
	// Stop and remove only this project's previous deployment if it exists
	fmt.Printf("[DOCKER] Cleaning up existing deployment for %s\n", deployment.ProjectName)
	cleanupCmd := exec.Command("docker", "compose", "down", "-v")
	cleanupCmd.Dir = workDir
	cleanupCmd.Stdout = os.Stdout
	cleanupCmd.Stderr = os.Stderr
	cleanupCmd.Run() // Ignore errors as containers might not exist

	// Create network if it doesn't exist
	fmt.Printf("[DOCKER] Ensuring deployment network exists\n")
	networkCmd := exec.Command("docker", "network", "create", "deployment-network")
	networkCmd.Stdout = os.Stdout
	networkCmd.Stderr = os.Stderr
	networkCmd.Run() // Ignore error if network already exists

	// Build and run using docker compose
	fmt.Printf("[DOCKER] Building and starting containers\n")
	cmd := exec.Command("docker", "compose", "up", "--build", "-d")
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *DockerSetup) configureNginx(deployment Deployment) error {
	configTemplate := `server {
    listen 80;
    listen 443 ssl;
    server_name %s.localhost;

    ssl_certificate /etc/nginx/ssl/server.crt;
    ssl_certificate_key /etc/nginx/ssl/server.key;
    ssl_trusted_certificate /etc/nginx/ssl/ca.crt;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:50m;
    ssl_session_tickets off;
    
    # HSTS (uncomment if you're sure)
    # add_header Strict-Transport-Security "max-age=63072000" always;

    # Redirect HTTP to HTTPS
    if ($scheme != "https") {
        return 301 https://$host$request_uri;
    }

    location / {
        proxy_pass http://localhost:%s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Add CORS headers
        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS' always;
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization' always;
        add_header 'Access-Control-Expose-Headers' 'Content-Length,Content-Range' always;
        
        # Handle preflight requests
        if ($request_method = 'OPTIONS') {
            add_header 'Access-Control-Max-Age' 1728000;
            add_header 'Content-Type' 'text/plain charset=UTF-8';
            add_header 'Content-Length' 0;
            return 204;
        }
    }
}`

	config := fmt.Sprintf(configTemplate, deployment.ProjectName, deployment.Port)
	configPath := fmt.Sprintf("/etc/nginx/sites-available/%s", deployment.ProjectName)
	symlinkPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s", deployment.ProjectName)

	// Write config using sudo
	tmpFile := fmt.Sprintf("/tmp/nginx_%s", deployment.ProjectName)
	if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write temporary config: %v", err)
	}

	// Move file to nginx directory using sudo
	if err := exec.Command("sudo", "mv", tmpFile, configPath).Run(); err != nil {
		return fmt.Errorf("failed to move nginx config: %v", err)
	}

	// Remove existing symlink if it exists
	exec.Command("sudo", "rm", "-f", symlinkPath).Run()

	// Create symlink using sudo
	if err := exec.Command("sudo", "ln", "-s", configPath, symlinkPath).Run(); err != nil {
		return fmt.Errorf("failed to create nginx symlink: %v", err)
	}

	// Test and reload nginx
	if err := exec.Command("sudo", "nginx", "-t").Run(); err != nil {
		return fmt.Errorf("nginx configuration test failed: %v", err)
	}

	if err := exec.Command("sudo", "systemctl", "reload", "nginx").Run(); err != nil {
		return fmt.Errorf("failed to reload nginx: %v", err)
	}

	return nil
}

// Improve isPortAvailable to check both Docker and system ports
func isPortAvailable(port string) bool {
	// Check if Docker is using the port
	dockerCmd := fmt.Sprintf("docker ps --format '{{.Ports}}' | grep ':%s->'", port)
	dockerErr := exec.Command("sh", "-c", dockerCmd).Run()

	// Check if system is using the port
	netstatCmd := fmt.Sprintf("netstat -tuln | grep LISTEN | grep :%s", port)
	netstatErr := exec.Command("sh", "-c", netstatCmd).Run()

	// Port is available if both commands return errors (port not found)
	return dockerErr != nil && netstatErr != nil
}
