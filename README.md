# MQTT_LoadTest
Load Testing tool for MQTT

## Prerequisites
- Go 1.21
- Docker

## Build
```bash
go build -o mqtt-load-tester main.go
```

## Usage
```bash
./mqtt-load-tester --clients 100 --rate 1000 --duration 60 --topic-prefix loadtest/ --username auth --password auth --qos 0 --prefix loadtest-client- --sub-clients 50 --retain

Usage of ./mqtt-load-tester:
      --broker string         MQTT broker URL (default "tcp://localhost:1883")
      --clients int           Number of concurrent clients (default 100)
      --duration int          Test duration in seconds (default 60)
      --password string       MQTT password (default "auth")
      --prefix string         Client ID prefix (default "loadtest-client-")
      --qos int               MQTT QoS level (0, 1, or 2)
      --rate int              Messages per second per client (default 1000)
      --retain                Retain published messages
      --size int              Size of each message in bytes (default 100)
      --sub-clients int       Number of subscribing clients (default 50)
      --topic-prefix string   Prefix for MQTT topics (default "loadtest/")
      --username string       MQTT username (default "auth")

# Pairwise mode (default) - each publisher paired with one subscriber
go run . --mode pairwise --clients 10 
# will be ignored --sub-clients 10

# N-to-1 mode - multiple publishers to one subscriber
go run . --mode ntone --clients 500 --sub-clients 100

# 1-to-N mode - one publisher to multiple subscribers
go run . --mode oton --clients 1 --sub-clients 100

# Many-to-many mode - all subscribers receive messages from all publishers
go run . --mode mtom --clients 10 --sub-clients 5
```

## Docker
```bash
docker run -d --name mosquitto -p 1883:1883 -p 9001:9001 eclipse-mosquitto
```