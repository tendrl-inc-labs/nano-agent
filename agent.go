package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type Config struct {
	AppURL           string
	LinuxPath        string
	SocketPath       string
	FlushInterval    time.Duration
	BatchSize        int
	ApiKey           string
	MinBatchSize     int
	MaxBatchSize     int
	ScaleFactor      float64
	MaxQueueSize     int
	TargetCPUPercent float64
	TargetMemPercent float64
	MinBatchInterval time.Duration
	MaxBatchInterval time.Duration
}

type MessageContext struct {
	Tags         []string    `json:"tags,omitempty"`
	Limit        interface{} `json:"-"`
	WaitResponse bool        `json:"wait,omitempty"`
	Entity       string      `json:"entity,omitempty"`
}

type Message struct {
	Data        string         `json:"data,omitempty"` //omitempty to allow check_msg with no data
	Context     MessageContext `json:"context,omitempty"`
	MsgType     string         `json:"msg_type,omitempty"`
	Destination string         `json:"dest,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type ResponseMessage struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type SystemMetrics struct {
	CPUUsage    float64
	MemoryUsage float64
	QueueLoad   float64 // Current queue size / max queue size
}

var (
	config       Config
	client       *http.Client
	messageQueue chan Message
	done         = make(chan struct{})
)

func InitializeConfig() {
	flag.StringVar(&config.ApiKey, "apiKey", "", "API key for authentication")
	flag.DurationVar(&config.FlushInterval, "flushInterval", 250*time.Millisecond, "Flush interval for batching")
	flag.IntVar(&config.BatchSize, "batchSize", 10, "Batch size for processing")
	flag.IntVar(&config.MinBatchSize, "minBatchSize", 10, "Minimum batch size")
	flag.IntVar(&config.MaxBatchSize, "maxBatchSize", 200, "Maximum batch size")
	flag.Float64Var(&config.ScaleFactor, "scaleFactor", 0.5, "Queue scale factor for batch size")
	flag.IntVar(&config.MaxQueueSize, "maxQueue", 1000, "Maximum queue size before backpressure")
	flag.Float64Var(&config.TargetCPUPercent, "targetCPU", 70.0, "Target CPU usage percentage")
	flag.Float64Var(&config.TargetMemPercent, "targetMem", 80.0, "Target memory usage percentage")
	flag.DurationVar(&config.MinBatchInterval, "minInterval", 100*time.Millisecond, "Minimum batch interval")
	flag.DurationVar(&config.MaxBatchInterval, "maxInterval", 1*time.Second, "Maximum batch interval")
	flag.Parse()

	if config.ApiKey == "" {
		config.ApiKey = os.Getenv("TENDRL_KEY")
		if config.ApiKey == "" {
			fmt.Println("Exiting: Missing API key")
			os.Exit(1)
		}
	}

	config.AppURL = "https://app.tendrl.com/api"

	// Set platform-appropriate defaults for Unix socket paths
	if runtime.GOOS == "windows" {
		config.LinuxPath = "C:\\ProgramData\\tendrl"
		config.SocketPath = config.LinuxPath + "\\tendrl_agent.sock"
		fmt.Printf("Windows detected: Using AF_UNIX socket at %s\n", config.SocketPath)
	} else {
		config.LinuxPath = "/var/lib/tendrl"
		config.SocketPath = config.LinuxPath + "/tendrl_agent.sock"
		fmt.Printf("Unix/Linux detected: Using AF_UNIX socket at %s\n", config.SocketPath)
	}
}

func ValidateClientContext(ctx *MessageContext) error {
	if ctx != nil && len(ctx.Tags) > 10 {
		return fmt.Errorf("too many tags provided; maximum is 10")
	}
	return nil
}

func HandleConnection(conn net.Conn) {
	defer conn.Close()
	decoder := json.NewDecoder(bufio.NewReader(conn))

	for {
		var msg Message
		if err := decoder.Decode(&msg); err == io.EOF {
			fmt.Println("Connection closed by client")
			break
		} else if err != nil {
			fmt.Printf("Error decoding JSON message: %v\n", err)
			continue
		}

		err := ValidateClientContext(&msg.Context)
		if err != nil {
			log.Print(err)
			sendErrorResponse(conn, err.Error())
			continue
		}

		ProcessMessage(conn, msg)
	}
}

func ProcessMessage(conn net.Conn, msg Message) {
	if len(msg.Context.Tags) > 0 {
		fmt.Printf("Processing message with tags: %v\n", msg.Context.Tags)
	}

	switch msg.MsgType {
	case "msg_check":
		limit := 1
		var ok bool
		if msg.Context.Limit != nil {
			limit, ok = msg.Context.Limit.(int)
			if !ok {
				sendErrorResponse(conn, "Invalid limit type")
				return
			}
		}

		messages, err := checkMessage(client, limit)
		if err != nil {
			sendErrorResponse(conn, err.Error())
			return
		}

		if len(messages) == 0 {
			conn.Write([]byte("204"))
			return
		}
		response, _ := json.Marshal(messages)
		conn.Write(response)

	case "publish":
		if msg.Context.WaitResponse {
			resp := sendSingleMessage(msg)
			response, _ := json.Marshal(resp)
			conn.Write(response)
			return
		}

		messageQueue <- msg

	default:
		sendErrorResponse(conn, "Unknown message type")
	}
}

func getSystemMetrics() SystemMetrics {
	var metrics SystemMetrics

	// Get CPU usage
	cpuPercent, err := cpu.Percent(100*time.Millisecond, false)
	if err == nil && len(cpuPercent) > 0 {
		metrics.CPUUsage = cpuPercent[0]
	}

	// Get memory usage
	vm, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryUsage = vm.UsedPercent
	}

	// Calculate queue load
	metrics.QueueLoad = float64(len(messageQueue)) / float64(config.MaxQueueSize) * 100

	return metrics
}

func calculateDynamicBatchSize(metrics SystemMetrics) int {
	// Reduce batch size if system is under pressure
	cpuFactor := math.Max(0, 1-(metrics.CPUUsage/config.TargetCPUPercent))
	memFactor := math.Max(0, 1-(metrics.MemoryUsage/config.TargetMemPercent))
	queueFactor := math.Min(1, metrics.QueueLoad/50) // Increase batch size if queue is filling up

	// Combine factors (weighted average)
	resourceFactor := (cpuFactor*0.4 + memFactor*0.4 + queueFactor*0.2)

	// Calculate new batch size
	newBatchSize := int(float64(config.MaxBatchSize) * resourceFactor)

	// Ensure we stay within bounds
	if newBatchSize < config.MinBatchSize {
		return config.MinBatchSize
	}
	if newBatchSize > config.MaxBatchSize {
		return config.MaxBatchSize
	}

	return newBatchSize
}

func ProcessQueue() {
	batch := make([]Message, 0, config.MaxBatchSize)
	ticker := time.NewTicker(config.MinBatchInterval)

	for {
		select {
		case msg := <-messageQueue:
			batch = append(batch, msg)

			// Get current system metrics
			metrics := getSystemMetrics()

			// Calculate dynamic batch size based on system load
			dynamicBatchSize := calculateDynamicBatchSize(metrics)

			// Adjust ticker interval based on system load
			interval := time.Duration(float64(config.MaxBatchInterval) *
				(1 - metrics.QueueLoad/100))
			if interval < config.MinBatchInterval {
				interval = config.MinBatchInterval
			}
			ticker.Reset(interval)

			if len(batch) >= dynamicBatchSize {
				FlushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				FlushBatch(batch)
				batch = batch[:0]
			}

		case <-done:
			close(messageQueue) // Prevent further writes
			for msg := range messageQueue {
				batch = append(batch, msg)
			}
			if len(batch) > 0 {
				FlushBatch(batch)
			}
			ticker.Stop()
			return
		}
	}
}

func FlushBatch(batch []Message) {
	payload, err := json.Marshal(batch)
	if err != nil {
		fmt.Printf("Error marshalling batch: %v\n", err)
		return
	}

	fmt.Printf("Flushing batch with %d messages...\n", len(batch))
	req, err := http.NewRequest("POST", config.AppURL+"/messages", bytes.NewBuffer(payload))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+config.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending batch: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Failed to send batch, status: %d, body: %s\n", resp.StatusCode, string(body))
	}
}

func sendErrorResponse(conn net.Conn, errorMsg string) {
	resp := ResponseMessage{
		Status:  "error",
		Message: errorMsg,
	}
	data, _ := json.Marshal(resp)
	conn.Write(data)
}

func main() {
	InitializeConfig()

	// Initialize message queue with configured size
	messageQueue = make(chan Message, config.MaxQueueSize)

	CreateDirs(config.LinuxPath)

	client = &http.Client{
		Timeout: 10 * time.Second,
	}

	// On Windows, check if AF_UNIX is supported
	if runtime.GOOS == "windows" {
		if !isWindowsAFUnixSupported() {
			fmt.Println("Error: AF_UNIX sockets not supported on this Windows version.")
			fmt.Println("Please upgrade to Windows 10 version 1803 or later.")
			os.Exit(1)
		}
	}

	// Remove existing socket file
	os.Remove(config.SocketPath)

	// Create Unix socket listener on all platforms
	listener, err := net.Listen("unix", config.SocketPath)
	if err != nil {
		fmt.Printf("[main] AF_UNIX Listener error: %v\n", err)
		if runtime.GOOS == "windows" {
			fmt.Println("Hint: Ensure Windows 10 1803+ and AF_UNIX driver is enabled")
			fmt.Println("Check with: sc query afunix")
		}
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("Agent listening on AF_UNIX socket: %s\n", config.SocketPath)

	go ProcessQueue()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		close(done)
		fmt.Println("[main] Shutting down gracefully...")
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-done:
				return
			default:
				fmt.Printf("[main] Accept error: %v\n", err)
				continue
			}
		}
		go HandleConnection(conn)
	}
}

// isWindowsAFUnixSupported checks if Windows supports AF_UNIX sockets
func isWindowsAFUnixSupported() bool {
	// Try to create a test Unix socket to verify support
	testSocket, err := net.Listen("unix", "test_afunix.sock")
	if err != nil {
		return false
	}
	testSocket.Close()
	os.Remove("test_afunix.sock")
	return true
}

func checkMessage(client *http.Client, limit int) ([]Message, error) {
	url := fmt.Sprintf("%s/entities/check_messages?limit=%d", config.AppURL, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.ApiKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Match Python's status code handling
	if resp.StatusCode == 204 {
		return nil, nil
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response struct {
		Messages []Message `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Messages, nil
}

func sendSingleMessage(msg Message) interface{} {
	payload, err := json.Marshal(msg)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}

	req, err := http.NewRequest("POST", config.AppURL+"/entities/message", bytes.NewBuffer(payload))
	if err != nil {
		return map[string]string{"error": err.Error()}
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.ApiKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	defer resp.Body.Close()

	var result interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}
