package main

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	rb, err := common.NewRingBuffer("/tmp/test.mmap")
	if err != nil {
		panic(err)
	}
	defer rb.Close()
	efd := rb.Eventfd()

	// Set non-blocking mode
	if err := unix.SetNonblock(efd, true); err != nil {
		fmt.Fprintf(os.Stderr, "[demo_plugin] SetNonblock error: %v\n", err)
		return
	}

	// Add processing statistics
	var processedCount int
	lastStatsTime := time.Now()
	lastWriteCount := rb.GetWriteCount()

	msgBuf := make([]byte, 4096)
	var tmp [8]byte

	// Drain pipe buffer
	for {
		_, err := unix.Read(efd, tmp[:])
		if err != nil {
			break
		}
	}

	// Use shorter sleep time
	const minSleepTime = time.Microsecond * 5
	const maxSleepTime = time.Microsecond * 5
	sleepTime := minSleepTime

	for {
		// Consume all readable messages from ringbuffer first
		processed := false
		emptyCount := 0
		for {
			msg, ok := rb.ReadMsg()
			if !ok {
				emptyCount++
				if emptyCount > 3 { // Exit inner loop after 3 consecutive empty reads
					break
				}
				continue
			}
			processed = true
			processedCount++
			emptyCount = 0

			// Print message content only for debugging
			if false { // Set to true to enable detailed logs
				fmt.Printf("[demo_plugin] Raw message (len=%d): %x\n", len(msg), msg)
			}

			// Use preallocated buffer
			if len(msg) > len(msgBuf) {
				msgBuf = make([]byte, len(msg))
			}
			copy(msgBuf, msg)

			var m map[string]interface{}
			err = json.Unmarshal(msgBuf[:len(msg)], &m)
			if err != nil {
				if false { // Set to true to enable error logs
					fmt.Printf("[demo_plugin] JSON unmarshal failed: %v\n", err)
				}
				continue
			}

			if false { // Set to true to enable message logs
				fmt.Printf("[demo_plugin] Read message: %v\n", m)
			}
		}

		// Print processing statistics every 5 seconds
		if time.Since(lastStatsTime) > 5*time.Second {
			currentWriteCount := rb.GetWriteCount()
			writeDelta := currentWriteCount - lastWriteCount

			fmt.Printf("[demo_plugin] Stats: processed %d messages in 5 seconds (written: %d)\n",
				processedCount, writeDelta)

			processedCount = 0
			lastStatsTime = time.Now()
			lastWriteCount = currentWriteCount
		}

		// Dynamically adjust sleep time
		if !processed {
			sleepTime = min(sleepTime*2, maxSleepTime)
			time.Sleep(sleepTime)
		} else {
			sleepTime = minSleepTime
		}

		// Ringbuffer is empty, wait for new event
		_, err := unix.Read(efd, tmp[:])
		if err != nil {
			if err == unix.EAGAIN {
				continue
			}
			if err == unix.EINTR {
				continue
			}
			if false { // Set to true to enable error logs
				fmt.Fprintf(os.Stderr, "[demo_plugin] eventfd/pipe read error: %v\n", err)
			}
			time.Sleep(time.Millisecond * 100)
			// Drain pipe buffer
			for {
				_, err := unix.Read(efd, tmp[:])
				if err != nil {
					break
				}
			}
		}
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
