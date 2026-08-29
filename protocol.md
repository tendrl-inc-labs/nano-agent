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
- Windows: `C:\ProgramData\tendrl\tendrl_agent.sock` (AF_UNIX socket, requires Windows 10 1803+)

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

| Field       | Type             | Description                         | Required |
| ----------- | ---------------- | ----------------------------------- | -------- |
| data        | object or string | Message payload (JSON object or string) | No       |
| context     | object           | Additional context for the message  | No       |
| msg_type    | string           | Type of message (see Message Types) | Yes      |
| dest        | string           | Destination identifier              | No       |
| timestamp   | string           | Message timestamp                   | No       |

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

Publishes a message to all subscribers or to a specific destination entity.

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
  "msg_type": "publish",
  "dest": "optional-entity-name"
}
```

**Response:**

- If `wait` is `false`: None (asynchronous)
- If `wait` is `true`: Response from the server

### 3. Heartbeat (`heartbeat`)

Sends a heartbeat message with system metrics.

**Request:**

```json
{
  "data": {
    "mem_free": 1024.0,
    "mem_total": 4096.0,
    "disk_free": 50000.0,
    "disk_size": 100000.0
  },
  "msg_type": "heartbeat"
}
```

### 4. State New (`state_new`)

Creates a new state entry for the entity.

**Request:**

```json
{
  "data": {
    "key": "value"
  },
  "msg_type": "state_new"
}
```

### 5. State Update (`state_update`)

Updates an existing state entry for the entity.

**Request:**

```json
{
  "data": {
    "key": "updated_value"
  },
  "msg_type": "state_update"
}
```

### 6. State Read (`state_read`)

Reads the entity's current state table.

**Request:**

```json
{
  "msg_type": "state_read"
}
```

**Response:**

```json
{
  "statusTable": {
    "key": "updated_value"
  }
}
```

## Error Response

Error responses are JSON objects with the following structure:

```json
{
  "status": "error",
  "message": "Error description"
}
```

## Usage Examples

### CLI Client

The `tendrl` binary wraps the socket protocol for shell scripts and manual testing. It ships alongside `tendrl-agent` and does not require an API key.

```bash
# Verify the agent is listening
tendrl ping

# Publish data
tendrl publish -data '{"temperature": 22.5, "unit": "celsius"}' -tags sensor,building-a

# Publish and wait for a server response
tendrl publish -data '{"temperature": 22.5}' -wait

# Send to a specific entity
tendrl publish -data '{"command": "reboot"}' -dest control-panel-01

# Poll for incoming messages
tendrl check -limit 5

# State table operations
tendrl state new -data '{"firmware": "1.2.0", "mode": "active"}'
tendrl state update -data '{"mode": "standby"}'
tendrl state read

# Send a heartbeat
tendrl heartbeat -data '{"mem_free": 1024.0, "mem_total": 4096.0}'
```

Use `-data @payload.json` to load JSON from a file. Override the socket path with `-socket` if needed.

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
import socket
import json

# Requires Windows 10 version 1803 or later for AF_UNIX support
sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect(r"C:\ProgramData\tendrl\tendrl_agent.sock")

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
sock.sendall(json.dumps(message).encode())

# Wait for response
response = sock.recv(4096)
print(response.decode())

sock.close()
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

| Configuration Option | Type             | Default                                | Description                                            |
| -------------------- | ---------------- | -------------------------------------- | ------------------------------------------------------ |
| `ApiKey`             | `string`         | `""`                                   | Authentication API key                                 |
| `FlushInterval`      | `time.Duration`  | `250ms`                                | Interval for flushing message batches                  |
| `BatchSize`          | `int`            | `10`                                   | Default number of messages per batch                   |
| `MinBatchSize`       | `int`            | `10`                                   | Minimum number of messages per batch                   |
| `MaxBatchSize`       | `int`            | `200`                                  | Maximum number of messages per batch                   |
| `ScaleFactor`        | `float64`        | `0.5`                                  | Queue scaling factor for dynamic batch sizing          |
| `MaxQueueSize`       | `int`            | `1000`                                 | Maximum message queue size                             |
| `TargetCPUPercent`   | `float64`        | `70.0`                                 | Target CPU usage percentage for dynamic batch sizing   |
| `TargetMemPercent`   | `float64`        | `80.0`                                 | Target memory usage percentage for dynamic batch sizing |
| `MinBatchInterval`   | `time.Duration`  | `100ms`                                | Minimum time between batch sends                       |
| `MaxBatchInterval`   | `time.Duration`  | `1s`                                   | Maximum time between batch sends                       |
| `AppURL`             | `string`         | `"https://app.tendrl.com/api"`         | API endpoint (flag: `-appURL`, env: `TENDRL_APP_URL`)  |
| `LinuxPath`          | `string`         | `"/var/lib/tendrl"`                    | Base path for agent files                              |
| `SocketPath`         | `string`         | `"/var/lib/tendrl/tendrl_agent.sock"`  | Unix socket path                                       |

### Configuration Environment Variables

| Environment Variable | Description                |
| -------------------- | -------------------------- |
| `TENDRL_KEY`         | API key for authentication |
| `TENDRL_APP_URL`     | API base URL override      |
