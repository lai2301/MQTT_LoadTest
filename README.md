# MQTT_LoadTest
Load Testing tool for MQTT


```bash
go run main.go --clients 100 --rate 1000 --duration 60 --topic-prefix loadtest/ --username auth --password auth --qos 0 --prefix loadtest-client- --sub-clients 50 --retain

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
```
