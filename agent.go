package main

import (
	"bufio"
	"bytes"
	"context"
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
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// version is set at build time via ldflags
var version = "dev"

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
	Tags         []string `json:"tags,omitempty"`
	Limit        *int     `json:"limit,omitempty"`
	WaitResponse bool     `json:"wait,omitempty"`
	Entity       string   `json:"entity,omitempty"`
}

type Message struct {
	Data        json.RawMessage `json:"data,omitempty"` // Accepts string or object, forwarded as-is
	Context     MessageContext  `json:"context,omitempty"`
	MsgType     string          `json:"msg_type,omitempty"`
	Destination string          `json:"dest,omitempty"`
	Source      string          `json:"source,omitempty"` // Used when receiving messages from check_messages
	Timestamp   string          `json:"timestamp,omitempty"`
}

type CheckMessage struct {
	Data      json.RawMessage `json:"data,omitempty"` // Accepts string or object
	Tags      []string        `json:"tags,omitempty"`
	MsgType   string          `json:"msg_type,omitempty"`
	Source    string          `json:"source,omitempty"`
	Timestamp string          `json:"timestamp,omitempty"`
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

// maxResponseBody limits HTTP response reads to 10MB to prevent memory exhaustion
const maxResponseBody = 10 << 20

var (
	config       Config
	client       *http.Client
	messageQueue chan Message
	cancel       context.CancelFunc
	ctx          context.Context
	wg           sync.WaitGroup
)

func InitializeConfig() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.StringVar(&config.ApiKey, "apiKey", "", "API key for authentication")
	flag.StringVar(&config.AppURL, "appURL", "", "API base URL (default: https://app.tendrl.com/api)")
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

	if *showVersion {
		fmt.Printf("tendrl-agent v%s\n", version)
		os.Exit(0)
	}

	fmt.Printf("Tendrl Nano Agent v%s\n", version)

	if config.ApiKey == "" {
		config.ApiKey = os.Getenv("TENDRL_KEY")
		if config.ApiKey == "" {
			fmt.Println("Exiting: Missing API key")
			os.Exit(1)
		}
	}

	if config.AppURL == "" {
		config.AppURL = os.Getenv("TENDRL_APP_URL")
		if config.AppURL == "" {
			config.AppURL = "https://app.tendrl.com/api"
		}
	}

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
	defer wg.Done()
	defer conn.Close()

	// Set an initial read deadline to prevent hung connections
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	decoder := json.NewDecoder(bufio.NewReader(conn))

	for {
		var msg Message
		if err := decoder.Decode(&msg); err == io.EOF {
			fmt.Println("Connection closed by client")
			break
		} else if err != nil {
			// Check if shutdown was requested
			if ctx.Err() != nil {
				break
			}
			fmt.Printf("Error decoding JSON message: %v\n", err)
			continue
		}

		// Reset deadline after each successful read
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

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
		if msg.Context.Limit != nil && *msg.Context.Limit > 0 {
			limit = *msg.Context.Limit
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

	case "state_read":
		result, err := readStateTable(client)
		if err != nil {
			sendErrorResponse(conn, err.Error())
			return
		}
		response, _ := json.Marshal(result)
		conn.Write(response)

	case "heartbeat":
		// Heartbeats always go directly to the single-message endpoint
		resp := sendSingleMessage(msg)
		response, _ := json.Marshal(resp)
		conn.Write(response)

	case "publish", "state_new", "state_update":
		if msg.Context.WaitResponse {
			resp := sendSingleMessage(msg)
			response, _ := json.Marshal(resp)
			conn.Write(response)
			return
		}

		select {
		case messageQueue <- msg:
			// Queued successfully
		default:
			sendErrorResponse(conn, "Queue full, try again later")
		}

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
	defer ticker.Stop()

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

		case <-ctx.Done():
			// Drain remaining queued messages before exit
			for {
				select {
				case msg := <-messageQueue:
					batch = append(batch, msg)
				default:
					if len(batch) > 0 {
						FlushBatch(batch)
					}
					return
				}
			}
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

	maxRetries := 3
	backoff := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("Retry attempt %d/%d after %v...\n", attempt, maxRetries-1, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				fmt.Printf("Shutdown requested, dropping batch of %d messages\n", len(batch))
				return
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, "POST", config.AppURL+"/entities/messages", bytes.NewReader(payload))
		if err != nil {
			fmt.Printf("Error creating request: %v\n", err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+config.ApiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return // Shutdown requested, don't log as error
			}
			fmt.Printf("Error sending batch: %v\n", err)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		resp.Body.Close()

		if resp.StatusCode == http.StatusCreated {
			return // Success
		}

		// Don't retry client errors (4xx) — only server/network errors
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			fmt.Printf("Failed to send batch, status: %d, body: %s\n", resp.StatusCode, string(body))
			return
		}

		fmt.Printf("Failed to send batch, status: %d, body: %s\n", resp.StatusCode, string(body))
	}

	fmt.Printf("Batch of %d messages dropped after %d retries\n", len(batch), maxRetries)
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

	// Initialize context for coordinated shutdown
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Initialize message queue with configured size
	messageQueue = make(chan Message, config.MaxQueueSize)

	if err := CreateDirs(config.LinuxPath); err != nil {
		fmt.Printf("Warning: directory setup issue: %v\n", err)
	}

	client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
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
		fmt.Println("[main] Shutting down gracefully...")
		cancel()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				// Wait for in-flight connections to finish
				wg.Wait()
				return
			default:
				fmt.Printf("[main] Accept error: %v\n", err)
				continue
			}
		}
		wg.Add(1)
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

func checkMessage(client *http.Client, limit int) ([]CheckMessage, error) {
	url := fmt.Sprintf("%s/entities/check_messages?limit=%d", config.AppURL, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.ApiKey)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&response); err != nil {
		return nil, err
	}

	checkMessages := make([]CheckMessage, 0, len(response.Messages))
	for _, message := range response.Messages {
		checkMessages = append(checkMessages, CheckMessage{
			Data:      message.Data,
			Tags:      message.Context.Tags,
			MsgType:   message.MsgType,
			Source:    message.Source,
			Timestamp: message.Timestamp,
		})
	}
	return checkMessages, nil
}

func sendSingleMessage(msg Message) interface{} {
	payload, err := json.Marshal(msg)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.AppURL+"/entities/message", bytes.NewReader(payload))
	if err != nil {
		return map[string]string{"error": err.Error()}
	}

	req.Header.Set("Authorization", "Bearer "+config.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	defer resp.Body.Close()

	var result interface{}
	json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&result)
	return result
}

func readStateTable(client *http.Client) (interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", config.AppURL+"/entities/status-table", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.ApiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}
