package main

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	ID           string
	client       mqtt.Client
	config       *Config
	stats        *Stats
	stopChan     chan struct{}
	isSubscriber bool
	messagesChan chan []byte
	topic        string
	clientIndex  int
}

type Stats struct {
	PublishedMessages uint64
	ReceivedMessages  uint64
	Errors            uint64
	LastPublished     map[int]time.Time // Track last publish time per client
	LastReceived      map[int]time.Time // Track last receive time per client
	ChannelStats      map[int]*ChannelStat
	mutex             sync.Mutex
}

type ChannelStat struct {
	Published   uint64
	Received    uint64
	MinRate     float64
	MaxRate     float64
	TotalRate   float64
	SampleCount uint64
}

func NewStats() *Stats {
	return &Stats{
		LastPublished: make(map[int]time.Time),
		LastReceived:  make(map[int]time.Time),
		ChannelStats:  make(map[int]*ChannelStat),
	}
}

func (s *Stats) UpdateChannelRate(channelID int, rate float64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stat, exists := s.ChannelStats[channelID]
	if !exists {
		stat = &ChannelStat{
			MinRate: rate,
			MaxRate: rate,
		}
		s.ChannelStats[channelID] = stat
	}

	stat.TotalRate += rate
	stat.SampleCount++

	if rate < stat.MinRate {
		stat.MinRate = rate
	}
	if rate > stat.MaxRate {
		stat.MaxRate = rate
	}
}

func NewClient(id int, config *Config, stats *Stats, isSubscriber bool) (*Client, error) {
	clientID := fmt.Sprintf("%s%d", config.ClientPrefix, id)
	topic := fmt.Sprintf("%s%d", config.TopicPrefix, id/2)

	opts := mqtt.NewClientOptions().
		AddBroker(config.BrokerURL).
		SetClientID(clientID).
		SetUsername(config.Username).
		SetPassword(config.Password).
		SetCleanSession(true).
		SetAutoReconnect(true)

	if isSubscriber {
		opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
			stats.mutex.Lock()
			stats.ReceivedMessages++
			stats.LastReceived[id/2] = time.Now()
			stats.mutex.Unlock()

			// Extract timestamp from payload for latency calculation
			if len(msg.Payload()) >= 8 {
				sentTime := string(msg.Payload()[:8])
				if timestamp, err := strconv.ParseInt(sentTime, 10, 64); err == nil {
					_ = time.Since(time.Unix(0, timestamp))
				}
			}
		})
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return &Client{
		ID:           clientID,
		client:       client,
		config:       config,
		stats:        stats,
		stopChan:     make(chan struct{}),
		isSubscriber: isSubscriber,
		messagesChan: make(chan []byte, 100),
		topic:        topic,
		clientIndex:  id / 2,
	}, nil
}

func (c *Client) Start() {
	if c.isSubscriber {
		go c.subscribeLoop()
	} else {
		go c.publishLoop()
	}
}

func (c *Client) subscribeLoop() {
	token := c.client.Subscribe(c.topic, byte(c.config.QoS), nil)
	if token.Wait() && token.Error() != nil {
		c.stats.mutex.Lock()
		c.stats.Errors++
		c.stats.mutex.Unlock()
		return
	}

	<-c.stopChan
}

func (c *Client) publishLoop() {
	ticker := time.NewTicker(time.Second / time.Duration(c.config.PublishRate))
	defer ticker.Stop()

	payload := make([]byte, c.config.MessageSize)
	lastCount := uint64(0)
	lastTime := time.Now()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			// Calculate and update rate every second
			if time.Since(lastTime) >= time.Second {
				c.stats.mutex.Lock()
				currentCount := c.stats.PublishedMessages
				rate := float64(currentCount-lastCount) / time.Since(lastTime).Seconds()
				c.stats.UpdateChannelRate(c.clientIndex, rate)
				lastCount = currentCount
				c.stats.mutex.Unlock()
				lastTime = time.Now()
			}

			_, err := rand.Read(payload)
			if err != nil {
				c.stats.mutex.Lock()
				c.stats.Errors++
				c.stats.mutex.Unlock()
				continue
			}

			// Add timestamp to payload for latency calculation
			timestamp := time.Now().UnixNano()
			copy(payload[0:8], []byte(fmt.Sprintf("%d", timestamp)))

			token := c.client.Publish(c.topic, byte(c.config.QoS), c.config.RetainMessage, payload)
			if token.Wait() && token.Error() != nil {
				c.stats.mutex.Lock()
				c.stats.Errors++
				c.stats.mutex.Unlock()
				continue
			}

			c.stats.mutex.Lock()
			c.stats.PublishedMessages++
			c.stats.LastPublished[c.clientIndex] = time.Now()
			c.stats.mutex.Unlock()
		}
	}
}

func (c *Client) Stop() {
	close(c.stopChan)
	if c.isSubscriber {
		c.client.Unsubscribe(c.topic)
	}
	c.client.Disconnect(250)
}
