# Tendrl Nano Agent Socket Protocol

## Overview

The Tendrl Nano Agent is a lightweight, secure communication gateway designed to provide a unified, language-agnostic interface for system-wide message routing and platform communication.

### Key Design Principles

1. **Unified Communication Hub**
   - Serves as a single point of ingress and egress for multiple applications on a host
   - Supports applications written in different programming languages
   - Enables seamless inter-application and platform communication

2. **Security and Authorization**
   - Centralized authentication using a single API key
   - Provides a secure, controlled communication channel
   - Eliminates the need for individual application-level authentication

3. **Architectural Flexibility**
   - Can be deployed as a Unix socket service on traditional systems
   - Supports use as a sidecar in Kubernetes environments
   - Minimal overhead with high-performance message processing

4. **Cross-Platform Compatibility**
   - Consistent communication protocol across different operating systems
   - Supports Unix-like systems and Windows
   - Language-agnostic socket-based interface

The agent acts as a lightweight, secure conduit, allowing diverse applications to communicate efficiently while maintaining a centralized, controlled communication strategy.

## Socket Connection

### Socket Location

- Linux/macOS: `/var/lib/tendrl/tendrl_agent.sock`
- Windows: Named pipe `\\.\pipe\tendrl_agent`

## Message Format

All messages are JSON-encoded. The basic message structure is:

```json
{
  "data": {
    "key": "value"
  },
  "context": {
    "tags": ["tag1", "tag2"],
    "wait": false,
    "entity": "string"
  },
  "msg_type": "string",
  "dest": "string",
  "timestamp": "string"
}
```

### Fields

| Field       | Type   | Description                                            | Required |
|-------------|--------|--------------------------------------------------------|----------|
| data        | object | Message payload                                        | No       |
| context     | object | Additional context for the message                     | No       |
| msg_type    | string | Type of message (see Message Types)                    | Yes      |
| dest        | string | Destination identifier                                 | No       |
| timestamp   | string | Message timestamp                                      | No       |

### Context Object

| Field       | Type    | Description                                          | Constraints |
|-------------|---------|------------------------------------------------------|-------------|
| tags        | array   | Tags for message categorization                      | Max 10 tags |
| wait        | boolean | Whether to wait for server response                  | No          |
| entity      | string  | Entity identifier                                    | No          |

## Message Types

### 1. Check Messages (`msg_check`)

Checks for pending messages from the server.

**Request:**

```json
{
  "msg_type": "msg_check",
  "context": {
    "limit": 1
  }
}
```

**Response:**

- If messages available: Array of message objects
- If no messages: `204` (No Content)

### 2. Publish Message (`publish`)

Publishes a message to all subscribers.

**Request:**

```json
{
  "data": {
    "action": "create",
    "resource": "user",
    "details": {
      "name": "John Doe",
      "email": "john@example.com"
    }
  },
  "context": {
    "tags": ["user", "registration"],
    "wait": false
  },
  "msg_type": "publish"
}
```

**Response:**

- If `wait` is `false`: None (asynchronous)
- If `wait` is `true`: Response from the server

## Error Response

Error responses are JSON objects with the following structure:

```json
{
  "status": "error",
  "message": "Error description"
}
```

## Usage Examples

### Connecting to the Socket (Unix/Linux)

```python
import socket
import json

sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect("/var/lib/tendrl/tendrl_agent.sock")

# Send a message with JSON data
message = {
  "msg_type": "publish",
  "data": {
    "action": "update",
    "resource": "configuration",
    "details": {
      "key": "logging_level",
      "value": "debug"
    }
  }
}
sock.sendall(json.dumps(message).encode())

# Wait for response
response = sock.recv(4096)
print(response.decode())

sock.close()
```

### Connecting to the Socket (Windows)

```python
import win32pipe
import win32file
import json

pipe = win32file.CreateFile(
    r'\\.\pipe\tendrl_agent',
    win32file.GENERIC_READ | win32file.GENERIC_WRITE,
    0, None, win32file.OPEN_EXISTING, 0, None
)

# Send a message with JSON data
message = {
  "msg_type": "publish",
  "data": {
    "action": "create",
    "resource": "event",
    "details": {
      "type": "system_alert",
      "severity": "warning"
    }
  }
}
win32file.WriteFile(pipe, json.dumps(message).encode())

# Wait for response
response = win32file.ReadFile(pipe, 4096)
print(response[1].decode())

win32file.CloseHandle(pipe)
```

## Implementation Notes

1. The maximum number of tags in a message context is 10
2. Messages may be queued and batched for delivery to improve performance
3. The socket connection is stateless - each connection can be closed after sending/receiving messages

## Error Handling

Common errors include:

- Too many tags provided (max 10)
- Unknown message type
- Invalid JSON format

## Client Configuration Table

| Configuration Option | Type | Default | Description |
|---------------------|------|---------|-------------|
| `ApiKey` | `string` | `""` | Authentication API key |
| `FlushInterval` | `time.Duration` | `250ms` | Interval for flushing message batches |
| `BatchSize` | `int` | `10` | Default number of messages per batch |
| `MinBatchSize` | `int` | `10` | Minimum number of messages per batch |
| `MaxBatchSize` | `int` | `200` | Maximum number of messages per batch |
| `ScaleFactor` | `float64` | `0.5` | Queue scaling factor for dynamic batch sizing |
| `MaxQueueSize` | `int` | `1000` | Maximum message queue size |
| `TargetCPUPercent` | `float64` | `70.0` | Target CPU usage percentage for dynamic batch sizing |
| `TargetMemPercent` | `float64` | `80.0` | Target memory usage percentage for dynamic batch sizing |
| `MinBatchInterval` | `time.Duration` | `100ms` | Minimum time between batch sends |
| `MaxBatchInterval` | `time.Duration` | `1s` | Maximum time between batch sends |
| `AppURL` | `string` | `"https://app.tendrl.com/api"` | Default API endpoint |
| `LinuxPath` | `string` | `"/var/lib/tendrl"` | Base path for agent files |
| `SocketPath` | `string` | `"/var/lib/tendrl/tendrl_agent.sock"` | Unix socket path |

### Configuration Environment Variables

| Environment Variable | Description |
|---------------------|-------------|
| `TENDRL_KEY` | API key for authentication |
| `TENDRL_APP_URL` | Custom API endpoint |
| `TENDRL_SOCKET_PATH` | Custom Unix socket path |
