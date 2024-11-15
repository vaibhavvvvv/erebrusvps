# Step 1: Use the official Golang image as the base
FROM golang:1.20-alpine

# Step 2: Set the working directory inside the container
WORKDIR /app

# Step 3: Copy the go.mod and go.sum files to the working directory
COPY go.mod go.sum ./

# Step 4: Download the Go module dependencies
RUN go mod download

# Step 5: Copy the rest of the application code to the working directory
COPY . .

# Step 6: Build the Go application
RUN go build -o main .

# Step 7: Expose the application's port (adjust as needed)
EXPOSE 8080

# Step 8: Set the entry point to run the application
CMD ["./main"]