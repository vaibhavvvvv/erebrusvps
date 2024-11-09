package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	// "encoding/json"
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
	fmt.Printf("\n[DEPLOY] Starting deployment for project: %s\n", deployment.ProjectName)

	// If no port specified, get next available port
	if deployment.Port == "" {
		deployment.Port = getNextAvailablePort()
		fmt.Printf("[DEPLOY] Assigned new port %s for project %s\n", deployment.Port, deployment.ProjectName)
	} else if !isPortAvailable(deployment.Port) {
		// If requested port is not available, get a new one
		deployment.Port = getNextAvailablePort()
		fmt.Printf("[DEPLOY] Requested port unavailable, using port %s for project %s\n", deployment.Port, deployment.ProjectName)
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
	fmt.Printf("[DEPLOY] Creating workspace directory: %s\n", workDir)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %v", err)
	}

	// Clone repository
	fmt.Printf("[DEPLOY] Cloning repository: %s\n", deployment.GitURL)
	if err := d.cloneRepository(deployment.GitURL, workDir); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %v", err)
	}

	// Create Dockerfile if it doesn't exist
	fmt.Printf("[DEPLOY] Ensuring Dockerfile exists\n")
	if err := d.ensureDockerfile(workDir); err != nil {
		return nil, fmt.Errorf("failed to create Dockerfile: %v", err)
	}

	// Create docker-compose.yml
	fmt.Printf("[DEPLOY] Creating docker-compose.yml\n")
	if err := d.createDockerCompose(workDir, deployment); err != nil {
		return nil, fmt.Errorf("failed to create docker-compose.yml: %v", err)
	}

	// Build and run the container
	fmt.Printf("[DEPLOY] Building and running containers\n")
	if err := d.buildAndRun(workDir, deployment); err != nil {
		return nil, fmt.Errorf("failed to build and run: %v", err)
	}

	// Configure Nginx reverse proxy
	fmt.Printf("[DEPLOY] Configuring Nginx reverse proxy\n")
	if err := d.configureNginx(deployment); err != nil {
		return nil, fmt.Errorf("failed to configure nginx: %v", err)
	}

	fmt.Printf("[DEPLOY] Deployment completed successfully!\n")
	fmt.Println(&DeploymentResult{
		Status: "success",
		URL:    fmt.Sprintf("http://%s.localhost", deployment.ProjectName),
		Port:   deployment.Port,
	})

	return &DeploymentResult{
		Status: "success",
		URL:    fmt.Sprintf("http://%s.localhost", deployment.ProjectName),
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
	// Stop and remove existing containers for this project
	fmt.Printf("[DOCKER] Cleaning up existing deployment\n")
	cleanupCmd := exec.Command("docker", "compose", "down", "-v")
	cleanupCmd.Dir = workDir
	cleanupCmd.Stdout = os.Stdout
	cleanupCmd.Stderr = os.Stderr
	cleanupCmd.Run() // Ignore errors as containers might not exist

	// Remove any existing containers using the same port
	checkPortCmd := fmt.Sprintf("docker ps -q --filter publish=%s", deployment.Port)
	output, err := exec.Command("sh", "-c", checkPortCmd).Output()
	if err == nil && len(output) > 0 {
		fmt.Printf("[DOCKER] Found existing container using port %s, stopping it\n", deployment.Port)
		stopCmd := fmt.Sprintf("docker stop $(docker ps -q --filter publish=%s)", deployment.Port)
		exec.Command("sh", "-c", stopCmd).Run()
		rmCmd := fmt.Sprintf("docker rm $(docker ps -aq --filter publish=%s)", deployment.Port)
		exec.Command("sh", "-c", rmCmd).Run()
	}

	// Create network if it doesn't exist
	fmt.Printf("[DOCKER] Creating network: deployment-network\n")
	networkCmd := exec.Command("docker", "network", "create", "deployment-network")
	networkCmd.Stdout = os.Stdout
	networkCmd.Stderr = os.Stderr
	networkCmd.Run() // Ignore error if network already exists

	// Build and run using docker compose
	fmt.Printf("[DOCKER] Building and starting containers\n")
	cmd := exec.Command("docker", "compose", "up", "--build", "-d", "--force-recreate")
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *DockerSetup) configureNginx(deployment Deployment) error {
	// Remove existing nginx configuration if it exists
	configPath := fmt.Sprintf("/etc/nginx/sites-available/%s", deployment.ProjectName)
	symlinkPath := fmt.Sprintf("/etc/nginx/sites-enabled/%s", deployment.ProjectName)

	// Remove existing symlink if it exists
	os.Remove(symlinkPath)

	configTemplate := `
server {
    listen 80;
    server_name %s.localhost;

    location / {
        proxy_pass http://localhost:%s;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        
        # Add WebSocket support
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Add CORS headers
        add_header 'Access-Control-Allow-Origin' '*';
        add_header 'Access-Control-Allow-Methods' 'GET, POST, OPTIONS';
        add_header 'Access-Control-Allow-Headers' 'DNT,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range';
    }
}`

	config := fmt.Sprintf(configTemplate, deployment.ProjectName, deployment.Port)

	// Write config file
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write nginx config: %v", err)
	}

	// Create symlink
	if err := os.Symlink(configPath, symlinkPath); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create nginx symlink: %v", err)
	}

	// Test nginx configuration
	if err := exec.Command("sudo", "nginx", "-t").Run(); err != nil {
		return fmt.Errorf("nginx configuration test failed: %v", err)
	}

	// Reload Nginx
	if err := exec.Command("sudo", "systemctl", "reload", "nginx").Run(); err != nil {
		return fmt.Errorf("failed to reload nginx: %v", err)
	}

	return nil
}

// Add this new function to check if a port is available
func isPortAvailable(port string) bool {
	cmd := fmt.Sprintf("netstat -tuln | grep LISTEN | grep :%s", port)
	err := exec.Command("sh", "-c", cmd).Run()
	return err != nil // If error, port is not in use
}
