# MQTT Load Testing Tool

A high-performance load testing tool for MQTT brokers, supporting various testing patterns and configurations.

## Features
- Multiple testing modes (pairwise, N-to-1, 1-to-N, many-to-many)
- Configurable QoS levels and message retention
- Detailed performance metrics and statistics
- TLS/SSL support
- Automatic reconnection handling
- Customizable client behaviors

## Prerequisites
- Go 1.21 or higher
- Docker (optional, for local broker testing)

## Installation
```bash
git clone https://github.com/yourusername/mqtt-load-tester
cd mqtt-load-tester
go build
```

## Usage

### Basic Command
```bash
./mqtt-load-tester [options]
```

### Test Modes

1. **Pairwise Mode** (Default)
   - Each publisher paired with one subscriber to a dedicated topic (will be ignored --sub-clients)
   ```bash
   ./mqtt-load-tester --mode pairwise --clients 10
   ```

2. **N-to-1 Mode**
   - Multiple publishers to one subscriber
   ```bash
   ./mqtt-load-tester --mode ntone --clients 5 --sub-clients 1
   ```

3. **1-to-N Mode**
   - One publisher to multiple subscribers
   ```bash
   ./mqtt-load-tester --mode oton --clients 1 --sub-clients 5
   ```

4. **Many-to-Many Mode**
   - All subscribers receive messages from all publishers
   ```bash
   ./mqtt-load-tester --mode mtom --clients 10 --sub-clients 5
   ```

### Configuration Options
```
--broker string            MQTT broker URL (default "tcp://localhost:1883")
--clients int             Number of publishers (default 1)
--sub-clients int         Number of subscribers (default 1)
--rate int               Messages per second per publisher (default 10)
--size int               Message size in bytes (default 100)
--duration int           Test duration in seconds (default 30)
--qos int                MQTT QoS level (0, 1, or 2)
--retain                 Retain messages (default false)
--topic string           Topic prefix (default "loadtest/")
--username string        MQTT username
--password string        MQTT password
```

### Advanced Options
```
--clean-session bool      Clean session on connect (default true)
--auto-reconnect bool     Auto reconnect on connection loss (default true)
--resume-subs bool        Resume subscriptions after reconnect (default true)
--order-matters bool      Maintain message order (default false)
--msg-channel-depth uint  Message channel buffer size (default 1000)
--insecure-skip-verify   Skip TLS certificate verification (default true)
```

## Example Basic Test
```bash

./mqtt-load-tester --broker tcp://localhost:1883 --mode pairwise --clients 5 --duration 60
./mqtt-load-tester --broker ssl://localhost:1883 --mode ntone --clients 5 --sub-clients 1 --duration 60
./mqtt-load-tester --broker tcp://localhost:1883 --mode oton --clients 1 --sub-clients 5 --duration 60
./mqtt-load-tester --broker ssl://localhost:1883 --mode mtom --clients 10 --sub-clients 5 --duration 60
```

## Example of Result
```bash
=== Final Test Results ===
Total Publishers: 2000
Total Subscribers: 2000
Message Rate Per Client: 10
Testing Mode: pairwise
Test Duration: 600.87 seconds

=== Message Statistics ===
Total Messages Published: 11875842
Total Messages Received: 11857338
Messages Lost: 18504 (0.16%)
Dropped Messages: 17645
Duplicate Messages: 18504
Out of Order Messages: 17611
Processing Errors: 0

=== Performance Metrics ===
Average Publishing Rate: 19764.27 msg/sec
Average Receiving Rate: 19733.48 msg/sec
Average Publishing Rate Per Channel: 9.88 msg/sec
Average Receiving Rate Per Channel: 9.87 msg/sec

=== Error Statistics ===
Total Errors: 0
Retry Attempts: 0
Successful Retries: 0
Timeout Errors: 0
Connection Errors: 0
```

## License

MIT License

Copyright (c) 2024 [LiamChan]

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.