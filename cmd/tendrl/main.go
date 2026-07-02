package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"tendrl/agent/internal/socketclient"
)

var version = "dev"

type messageContext struct {
	Tags   []string `json:"tags,omitempty"`
	Limit  *int     `json:"limit,omitempty"`
	Wait   bool     `json:"wait,omitempty"`
	Entity string   `json:"entity,omitempty"`
}

type message struct {
	Data    json.RawMessage `json:"data,omitempty"`
	Context messageContext  `json:"context,omitempty"`
	MsgType string          `json:"msg_type"`
	Dest    string          `json:"dest,omitempty"`
}

func main() {
	fs := flag.NewFlagSet("tendrl", flag.ExitOnError)
	socketPath := fs.String("socket", socketclient.DefaultSocketPath(), "Agent unix socket path")
	showVersion := fs.Bool("version", false, "Print version and exit")
	fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Printf("tendrl v%s\n", version)
		os.Exit(0)
	}

	args := fs.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "ping":
		err = runPing(*socketPath, args[1:])
	case "publish":
		err = runPublish(*socketPath, args[1:])
	case "check":
		err = runCheck(*socketPath, args[1:])
	case "heartbeat":
		err = runHeartbeat(*socketPath, args[1:])
	case "state":
		err = runState(*socketPath, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runPing(socketPath string, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("ping takes no arguments")
	}
	return socketclient.Ping(socketPath)
}

func runPublish(socketPath string, args []string) error {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	data := fs.String("data", "", "Message payload as JSON (or @file.json)")
	dest := fs.String("dest", "", "Destination entity")
	entity := fs.String("entity", "", "Entity identifier")
	tags := fs.String("tags", "", "Comma-separated tags")
	wait := fs.Bool("wait", false, "Wait for server response")
	fs.Parse(args)

	payload, err := parseData(*data)
	if err != nil {
		return err
	}

	msg := message{
		Data:    payload,
		MsgType: "publish",
		Dest:    *dest,
		Context: messageContext{
			Tags:   splitTags(*tags),
			Wait:   *wait,
			Entity: *entity,
		},
	}

	resp, err := socketclient.Send(socketPath, msg, *wait)
	if err != nil {
		return err
	}
	return outputResponse(resp)
}

func runCheck(socketPath string, args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	limit := fs.Int("limit", 1, "Maximum messages to retrieve")
	fs.Parse(args)

	msg := message{
		MsgType: "msg_check",
		Context: messageContext{
			Limit: limit,
		},
	}

	resp, err := socketclient.Send(socketPath, msg, true)
	if err != nil {
		return err
	}
	return outputResponse(resp)
}

func runHeartbeat(socketPath string, args []string) error {
	fs := flag.NewFlagSet("heartbeat", flag.ExitOnError)
	data := fs.String("data", "", "Heartbeat payload as JSON (or @file.json)")
	fs.Parse(args)

	payload, err := parseData(*data)
	if err != nil {
		return err
	}
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}

	msg := message{
		Data:    payload,
		MsgType: "heartbeat",
	}

	resp, err := socketclient.Send(socketPath, msg, true)
	if err != nil {
		return err
	}
	return outputResponse(resp)
}

func runState(socketPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("state requires a subcommand: read, new, or update")
	}

	switch args[0] {
	case "read":
		return runStateRead(socketPath, args[1:])
	case "new":
		return runStateWrite(socketPath, "state_new", args[1:])
	case "update":
		return runStateWrite(socketPath, "state_update", args[1:])
	default:
		return fmt.Errorf("unknown state subcommand: %s", args[0])
	}
}

func runStateRead(socketPath string, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("state read takes no arguments")
	}

	msg := message{MsgType: "state_read"}
	resp, err := socketclient.Send(socketPath, msg, true)
	if err != nil {
		return err
	}
	return outputResponse(resp)
}

func runStateWrite(socketPath, msgType string, args []string) error {
	fs := flag.NewFlagSet(msgType, flag.ExitOnError)
	data := fs.String("data", "", "State payload as JSON (or @file.json)")
	wait := fs.Bool("wait", false, "Wait for server response")
	fs.Parse(args)

	payload, err := parseData(*data)
	if err != nil {
		return err
	}
	if payload == nil {
		return fmt.Errorf("-data is required")
	}

	msg := message{
		Data:    payload,
		MsgType: msgType,
		Context: messageContext{
			Wait: *wait,
		},
	}

	resp, err := socketclient.Send(socketPath, msg, *wait)
	if err != nil {
		return err
	}
	return outputResponse(resp)
}

func parseData(value string) (json.RawMessage, error) {
	if value == "" {
		return nil, nil
	}

	source := value
	if strings.HasPrefix(value, "@") {
		contents, err := os.ReadFile(strings.TrimPrefix(value, "@"))
		if err != nil {
			return nil, fmt.Errorf("read data file: %w", err)
		}
		source = string(contents)
	}

	if !json.Valid([]byte(source)) {
		return nil, fmt.Errorf("invalid JSON data")
	}
	return json.RawMessage(source), nil
}

func splitTags(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func outputResponse(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if string(data) == "204" {
		return nil
	}

	var errResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &errResp) == nil && errResp.Status == "error" {
		if errResp.Message != "" {
			return fmt.Errorf("%s", errResp.Message)
		}
		return fmt.Errorf("agent error")
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		fmt.Println(string(data))
		return nil
	}

	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		fmt.Println(string(data))
		return nil
	}
	fmt.Println(string(pretty))
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `tendrl - socket client for the Tendrl Nano Agent

Usage:
  tendrl [flags] <command> [command flags]

Flags:
  -socket string
        Agent unix socket path (default %q)
  -version
        Print version and exit

Commands:
  ping
  publish   -data JSON [-tags a,b] [-dest name] [-entity id] [-wait]
  check     [-limit N]
  heartbeat [-data JSON]
  state read
  state new     -data JSON [-wait]
  state update  -data JSON [-wait]

Examples:
  tendrl ping
  tendrl publish -data '{"temperature": 22.5}' -tags sensor
  tendrl check -limit 10
  tendrl state read
`, socketclient.DefaultSocketPath())
}
