# Tendrl Nano Agent

[![Version](https://img.shields.io/badge/version-0.1.0-blue.svg)](https://github.com/tendrl-inc/clients/nano_agent)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/doc/devel/release.html)
[![License](https://img.shields.io/badge/license-Proprietary-red.svg)](LICENSE)

A lightweight, resource-efficient agent for the Tendrl messaging system written in Go.

## ⚠️ License Notice

**This software is licensed for use with Tendrl services only.**

### ✅ Allowed

- Use the software with Tendrl services
- Inspect and learn from the code for educational purposes
- Modify or extend the software for personal or Tendrl-related use

### ❌ Not Allowed

- Use in any competing product or service
- Connect to any backend not operated by Tendrl, Inc.
- Package into any commercial or hosted product (e.g., SaaS, PaaS)
- Copy design patterns or protocol logic for another system without permission

For licensing questions, contact: `support@tendrl.com`

See the [LICENSE](LICENSE.md) file for complete terms and restrictions.

## Features

- Cross-platform UNIX socket communication (Windows 10 1803+, Linux, macOS)
- Dynamic batch processing based on system resources
- Automatic resource monitoring (CPU, Memory, Queue load)
- Configurable batch sizes and intervals
- Graceful shutdown handling
- Secure socket permissions

## AF_UNIX Socket Requirements

### Windows

- **Minimum Version**: Windows 10 version 1803 (April 2018 Update) or later
- **Windows Server**: 2019 or later
- **Driver**: AF_UNIX support via `afunix.sys` kernel driver
- **Verification**: Run `sc query afunix` to verify driver is available

### Unix/Linux/macOS

- Native AF_UNIX support (all modern versions)

## Installation

### Quick Start (Recommended)

Download the latest pre-built binary for your platform from the [GitHub releases page](https://github.com/tendrl-inc/clients/nano_agent/releases):

#### Linux (x86_64)
```bash
# Download and install
curl -L -o tendrl-agent https://github.com/tendrl-inc/clients/nano_agent/releases/latest/download/tendrl-agent-linux-amd64
chmod +x tendrl-agent
sudo mv tendrl-agent /usr/local/bin/

# Create required directories and permissions
sudo mkdir -p /var/lib/tendrl
sudo groupadd tendrl
sudo chown :tendrl /var/lib/tendrl
sudo chmod 770 /var/lib/tendrl
```

#### Linux (ARM64)
```bash
curl -L -o tendrl-agent https://github.com/tendrl-inc/clients/nano_agent/releases/latest/download/tendrl-agent-linux-arm64
chmod +x tendrl-agent
sudo mv tendrl-agent /usr/local/bin/
```

#### macOS (Intel)
```bash
curl -L -o tendrl-agent https://github.com/tendrl-inc/clients/nano_agent/releases/latest/download/tendrl-agent-darwin-amd64
chmod +x tendrl-agent
sudo mv tendrl-agent /usr/local/bin/
```

#### macOS (Apple Silicon)
```bash
curl -L -o tendrl-agent https://github.com/tendrl-inc/clients/nano_agent/releases/latest/download/tendrl-agent-darwin-arm64
chmod +x tendrl-agent
sudo mv tendrl-agent /usr/local/bin/
```

#### Windows (x86_64)
```powershell
# Download using PowerShell
Invoke-WebRequest -Uri "https://github.com/tendrl-inc/clients/nano_agent/releases/latest/download/tendrl-agent-windows-amd64.exe" -OutFile "tendrl-agent.exe"

# Create required directories
mkdir "C:\ProgramData\tendrl"
```

### Manual Installation (From Source)

If you prefer to build from source, you'll need Go 1.23+ installed:

```bash
git clone https://github.com/tendrl-inc/clients/nano_agent.git
cd nano_agent
go build -o tendrl-agent .
```

### Windows (10 1803+)

```bash
# Create required directories
mkdir "C:\ProgramData\tendrl"

# Start agent
tendrl-agent.exe -apiKey=YOUR_API_KEY
```

### Unix/Linux

```bash
# Create required directories and permissions
sudo mkdir -p /var/lib/tendrl
sudo groupadd tendrl
sudo chown :tendrl /var/lib/tendrl
sudo chmod 770 /var/lib/tendrl
```

## Configuration

### Command Line Options

```bash
./tendrl-agent \
  -apiKey=YOUR_API_KEY \
  -appURL=https://app.tendrl.com/api \
  -minBatchSize=10 \
  -maxBatchSize=200 \
  -targetCPU=70.0 \
  -targetMem=80.0 \
  -flushInterval=250ms \
  -maxQueue=1000
```

### Environment Variables

```bash
export TENDRL_KEY=your_api_key
export TENDRL_APP_URL=https://app.tendrl.com/api  # optional
./tendrl-agent
```

### Configuration Options

| Option         | Type     | Default                        | Description                       |
| -------------- | -------- | ------------------------------ | --------------------------------- |
| apiKey         | string   | -                              | API key (env: `TENDRL_KEY`)       |
| appURL         | string   | `https://app.tendrl.com/api`   | API base URL (env: `TENDRL_APP_URL`) |
| minBatchSize   | int      | 10                             | Minimum messages per batch        |
| maxBatchSize   | int      | 200                            | Maximum messages per batch        |
| targetCPU      | float    | 70.0                           | Target CPU usage percentage       |
| targetMem      | float    | 80.0                           | Target memory usage percentage    |
| flushInterval  | duration | 250ms                          | Maximum time between flushes      |
| maxQueue       | int      | 1000                           | Maximum messages in queue         |

## Message Format

For complete protocol specification and detailed documentation, see [`protocol.md`](protocol.md).

### Basic Message

```json
{
  "msg_type": "publish",
  "data": {
    "key": "value"
  },
  "context": {
    "tags": ["tag1", "tag2"],
    "wait": false,
    "entity": "optional-entity-id"
  },
  "dest": "optional-destination-entity"
}
```

The `data` field accepts either a JSON object or a string.

### Message Types

- `publish`: Standard message publishing (with optional destination)
- `msg_check`: Check for incoming messages
- `heartbeat`: Send system metrics (mem_free, mem_total, disk_free, disk_size)
- `state_new`: Create a new state entry for the entity
- `state_update`: Update an existing state entry
- `state_read`: Read the entity's current state table

## Usage Examples

### All Platforms (AF_UNIX Socket)

1. Send a simple message:

**Unix/Linux/macOS:**

```bash
echo '{"msg_type": "publish", "data": {"temperature": 22.5}, "context": {"tags": ["sensor"]}}' | \
  nc -U /var/lib/tendrl/tendrl_agent.sock
```

**Windows:**

```powershell
echo '{"msg_type": "publish", "data": {"temperature": 22.5}, "context": {"tags": ["sensor"]}}' | `
  nc -U "C:\ProgramData\tendrl\tendrl_agent.sock"
```

2. Send with wait response:

**Unix/Linux/macOS:**

```bash
echo '{"msg_type": "publish", "data": {"status": "ok"}, "context": {"wait": true}}' | \
  nc -U /var/lib/tendrl/tendrl_agent.sock
```

**Windows:**

```powershell
echo '{"msg_type": "publish", "data": {"status": "ok"}, "context": {"wait": true}}' | `
  nc -U "C:\ProgramData\tendrl\tendrl_agent.sock"
```

3. Check for messages:

**Unix/Linux/macOS:**

```bash
echo '{"msg_type": "msg_check"}' | nc -U /var/lib/tendrl/tendrl_agent.sock
```

**Windows:**

```powershell
echo '{"msg_type": "msg_check"}' | nc -U "C:\ProgramData\tendrl\tendrl_agent.sock"
```

## Service Installation

### Unix/Linux - Systemd

```ini
[Unit]
Description=Tendrl Nano Agent
After=network.target

[Service]
Type=simple
Environment=TENDRL_KEY=your_api_key
ExecStart=/usr/local/bin/tendrl-agent
Restart=always
User=tendrl
Group=tendrl

[Install]
WantedBy=multi-user.target
```

### Windows - Service

```powershell
# Install as Windows service using NSSM or similar
nssm install TendrlAgent "C:\Program Files\Tendrl\tendrl-agent.exe"
nssm set TendrlAgent Environment "TENDRL_KEY=your_api_key"
nssm start TendrlAgent
```

### Service Management

#### Unix/Linux

```bash
# Install service
sudo cp tendrl-agent.service /etc/systemd/system/
sudo systemctl daemon-reload

# Start service
sudo systemctl start tendrl-agent

# Enable on boot
sudo systemctl enable tendrl-agent

# Check status
sudo systemctl status tendrl-agent
```

#### Windows

```powershell
# Check service status
sc query TendrlAgent

# Start service
sc start TendrlAgent

# Stop service
sc stop TendrlAgent
```

## Troubleshooting

### Windows AF_UNIX Issues

1. AF_UNIX Not Supported

```powershell
# Check if AF_UNIX driver is available
sc query afunix

# If not available, ensure Windows 10 1803+ or Windows Server 2019+
winver
```

2. Socket Permission Denied

```powershell
# Check directory permissions
icacls "C:\ProgramData\tendrl"

# Ensure write access to socket directory
```

### Unix/Linux Issues

1. Permission Denied

```bash
# Fix socket permissions
sudo chown :tendrl /var/lib/tendrl/tendrl_agent.sock
sudo chmod 660 /var/lib/tendrl/tendrl_agent.sock
```

2. Connection Refused

```bash
# Check if agent is running
systemctl status tendrl-agent

# Check socket exists
ls -l /var/lib/tendrl/tendrl_agent.sock
```

### Common Issues

3. Message Queue Full

```bash
# Check agent logs
# Unix: journalctl -u tendrl-agent -f
# Windows: Check Event Viewer or agent console

# Increase queue size
./tendrl-agent -maxQueue=20000
```

### Logging

#### Unix/Linux

The agent logs to systemd journal by default:

```bash
# View all logs
journalctl -u tendrl-agent

# Follow new logs
journalctl -u tendrl-agent -f

# View errors only
journalctl -u tendrl-agent -p err
```

#### Windows

Check Windows Event Viewer under Applications or run agent in console mode for direct output.

## Security Considerations

### Socket Security (All Platforms)

- Socket files created with restricted permissions
- Directory access controls prevent unauthorized access
- AF_UNIX provides better security than TCP for local IPC

### API Key Protection

- Store API key in environment variable
- Use service environment configuration
- Don't pass key on command line

### Network Security

- Agent uses HTTPS for API communication
- Certificate verification enabled by default
- AF_UNIX eliminates network-based local attacks
