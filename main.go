package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

func main() {
	config := LoadConfig()

	// Ensure we have equal numbers of publishers and subscribers
	config.NumClients = config.SubClients

	stats := NewStats()

	// Create progress bar
	bar := progressbar.NewOptions(config.TestDuration,
		progressbar.OptionSetDescription("Running test"),
		progressbar.OptionSetWidth(50),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("s"),
	)

	// Create publisher clients
	publishers := make([]*Client, config.NumClients)
	subscribers := make([]*Client, config.SubClients)
	var wg sync.WaitGroup

	// Create and connect subscriber clients first
	fmt.Printf("Initializing %d pairs of publishers and subscribers...\n", config.SubClients)
	for i := 0; i < config.SubClients; i++ {
		client, err := NewClient(i*2, config, stats, true)
		if err != nil {
			fmt.Printf("Error creating client pair %d: %v\n", i, err)
			continue
		}
		subscribers[i] = client
		wg.Add(1)

		go func(c *Client) {
			defer wg.Done()
			c.Start()
		}(client)
	}

	// Increase wait time for subscribers to connect and subscribe
	time.Sleep(3 * time.Second)

	// Create and connect publisher clients
	fmt.Printf("Creating %d publisher clients...\n", config.NumClients)
	for i := 0; i < config.NumClients; i++ {
		client, err := NewClient(i*2+1, config, stats, false)
		if err != nil {
			fmt.Printf("Error creating publisher client %d: %v\n", i, err)
			continue
		}
		publishers[i] = client
		wg.Add(1)

		go func(c *Client) {
			defer wg.Done()
			c.Start()
		}(client)
	}

	// Set synchronized start time for all publishers
	publishStartTime := time.Now().Add(time.Second)
	for _, pub := range publishers {
		if pub != nil {
			pub.SetStartTime(publishStartTime)
		}
	}

	// Wait a moment for all clients to be ready
	time.Sleep(time.Second)
	
	// Only start counting duration after all clients are initialized
	startTime := time.Now()
	lastStatsTime := time.Now()
	statsInterval := time.Second * 5 // Print stats every 5 seconds

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Start statistics reporting
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(startTime).Seconds()
			_ = bar.Set(int(elapsed))

			messageRate := float64(stats.PublishedMessages) / elapsed

			// Print statistics every 5 seconds
			if time.Since(lastStatsTime) >= statsInterval {
				lastStatsTime = time.Now()
				fmt.Printf("\n=== Test Statistics (%.1f seconds elapsed) ===\n", elapsed)
				fmt.Printf("Messages: Pub=%d, Sub=%d, Rate=%.2f msg/sec, Errors=%d\n",
					stats.PublishedMessages, stats.ReceivedMessages, messageRate, stats.Errors)
			}

			if int(elapsed) >= config.TestDuration {
				goto cleanup
			}

		case <-sigChan:
			fmt.Println("\nReceived interrupt signal, shutting down...")
			goto cleanup
		}
	}

cleanup:
	// Stop all clients
	for _, client := range publishers {
		if client != nil {
			client.Stop()
		}
	}
	for _, client := range subscribers {
		if client != nil {
			client.Stop()
		}
	}

	wg.Wait()

	// Print final statistics
	elapsed := time.Since(startTime).Seconds()
	fmt.Printf("\n=== Final Test Results ===\n")
	fmt.Printf("Test Duration: %.2f seconds\n", elapsed)
	fmt.Printf("Total Messages Published: %d\n", stats.PublishedMessages)
	fmt.Printf("Total Messages Received: %d\n", stats.ReceivedMessages)
	fmt.Printf("Message Loss: %.2f%%\n",
		100-float64(stats.ReceivedMessages)/float64(stats.PublishedMessages)*100)
	fmt.Printf("Average Publishing Rate: %.2f msg/sec\n",
		float64(stats.PublishedMessages)/elapsed)
	fmt.Printf("Average Receiving Rate: %.2f msg/sec\n",
		float64(stats.ReceivedMessages)/elapsed)
	fmt.Printf("Average Publishing Rate Per Channel: %.2f msg/sec\n",
		float64(stats.PublishedMessages)/(elapsed*float64(config.NumClients)))
	fmt.Printf("Average Receiving Rate Per Channel: %.2f msg/sec\n",
		float64(stats.ReceivedMessages)/(elapsed*float64(config.SubClients)))
	fmt.Printf("Total Errors: %d\n", stats.Errors)
}
