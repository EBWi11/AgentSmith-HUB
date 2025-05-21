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

	// 设置非阻塞模式
	if err := unix.SetNonblock(efd, true); err != nil {
		fmt.Fprintf(os.Stderr, "[demo_plugin] SetNonblock error: %v\n", err)
		return
	}

	// 添加处理统计
	var processedCount int
	lastStatsTime := time.Now()
	lastWriteCount := rb.GetWriteCount()

	// 预分配缓冲区
	msgBuf := make([]byte, 4096)
	var tmp [8]byte

	// 清空管道缓冲区
	for {
		_, err := unix.Read(efd, tmp[:])
		if err != nil {
			break
		}
	}

	// 使用更短的休眠时间
	const minSleepTime = time.Millisecond * 1
	const maxSleepTime = time.Millisecond * 5
	sleepTime := minSleepTime

	for {
		// 优先消费 ringbuffer 中所有可读消息
		processed := false
		emptyCount := 0
		for {
			msg, ok := rb.ReadMsg()
			if !ok {
				emptyCount++
				if emptyCount > 3 { // 连续3次空读取后退出内循环
					break
				}
				continue
			}
			processed = true
			processedCount++
			emptyCount = 0

			// 只在调试时打印消息内容
			if false { // 设置为 true 开启详细日志
				fmt.Printf("[demo_plugin] Raw message (len=%d): %x\n", len(msg), msg)
			}

			// 使用预分配的缓冲区
			if len(msg) > len(msgBuf) {
				msgBuf = make([]byte, len(msg))
			}
			copy(msgBuf, msg)

			var m map[string]interface{}
			err = json.Unmarshal(msgBuf[:len(msg)], &m)
			if err != nil {
				if false { // 设置为 true 开启错误日志
					fmt.Printf("[demo_plugin] JSON unmarshal failed: %v\n", err)
				}
				continue
			}

			if false { // 设置为 true 开启消息日志
				fmt.Printf("[demo_plugin] Read message: %v\n", m)
			}
		}

		// 每5秒打印一次处理统计
		if time.Since(lastStatsTime) > 5*time.Second {
			currentWriteCount := rb.GetWriteCount()
			writeDelta := currentWriteCount - lastWriteCount

			fmt.Printf("[demo_plugin] 处理统计: 5秒内处理了 %d 条消息 (写入: %d)\n",
				processedCount, writeDelta)

			processedCount = 0
			lastStatsTime = time.Now()
			lastWriteCount = currentWriteCount
		}

		// 动态调整休眠时间
		if !processed {
			sleepTime = min(sleepTime*2, maxSleepTime)
			time.Sleep(sleepTime)
		} else {
			sleepTime = minSleepTime
		}

		// ringbuffer 已空，等待新事件
		_, err := unix.Read(efd, tmp[:])
		if err != nil {
			if err == unix.EAGAIN {
				continue
			}
			if err == unix.EINTR {
				continue
			}
			if false { // 设置为 true 开启错误日志
				fmt.Fprintf(os.Stderr, "[demo_plugin] eventfd/pipe read error: %v\n", err)
			}
			time.Sleep(time.Millisecond * 100)
			// 清空管道缓冲区
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
