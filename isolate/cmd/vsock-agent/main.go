// vsock-agent is a simple command execution server that runs inside the VM.
// It listens on vsock port 1024 and executes commands received from the host.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/mdlayher/vsock"
)

const (
	// DefaultPort is the port we listen on
	DefaultPort = 1024
)

// Request is the JSON structure for incoming commands
type Request struct {
	Command string            `json:"command"`
	Stdin   string            `json:"stdin,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	WorkDir string            `json:"workdir,omitempty"`
}

// Response is the JSON structure for command output
type Response struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func main() {
	fmt.Println("[vsock-agent] Starting vsock server on port", DefaultPort)

	// Create vsock listener using the proper vsock library
	listener, err := vsock.Listen(DefaultPort, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vsock-agent] Failed to listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("[vsock-agent] Listening for connections...")

	for {
		// Accept connection
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[vsock-agent] Accept error: %v\n", err)
			continue
		}

		// Handle connection
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("[vsock-agent] Client connected")

	// Read length-prefixed message (4-byte big-endian length + JSON)
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		fmt.Fprintf(os.Stderr, "[vsock-agent] Read length error: %v\n", err)
		return
	}

	if length > 10*1024*1024 { // 10MB max
		sendResponse(conn, Response{Error: "request too large"})
		return
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		fmt.Fprintf(os.Stderr, "[vsock-agent] Read data error: %v\n", err)
		return
	}

	// Parse request
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		sendResponse(conn, Response{Error: fmt.Sprintf("invalid JSON: %v", err)})
		return
	}

	fmt.Printf("[vsock-agent] Executing: %s\n", req.Command)

	// Execute command
	resp := executeCommand(req)

	// Send response
	sendResponse(conn, resp)
}

func executeCommand(req Request) Response {
	cmd := exec.Command("/bin/sh", "-c", req.Command)

	// Set environment
	if len(req.Env) > 0 {
		env := os.Environ()
		for k, v := range req.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Set working directory
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	// Set stdin
	if req.Stdin != "" {
		cmd.Stdin = bytes.NewReader([]byte(req.Stdin))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err := cmd.Run()

	resp := Response{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = 1
			resp.Error = err.Error()
		}
	}

	return resp
}

func sendResponse(conn net.Conn, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		data = []byte(`{"error": "failed to marshal response"}`)
	}

	// Write length prefix
	length := uint32(len(data))
	binary.Write(conn, binary.BigEndian, length)

	// Write data
	conn.Write(data)
}
