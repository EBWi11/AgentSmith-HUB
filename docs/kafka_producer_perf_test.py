#!/usr/bin/env python3
"""
Kafka Producer Performance Test Script
Based on AgentSmith-HUB Performance Testing Report

This script generates and sends test messages to Kafka for performance testing.
Supports high-throughput message generation with configurable QPS.
"""

import json
import time
import random
import argparse
import threading
from concurrent.futures import ThreadPoolExecutor
from kafka import KafkaProducer
from kafka.errors import KafkaError
import signal
import sys

# Default message template based on performance test document
DEFAULT_MESSAGE = {
    "name": "John Doe",
    "age": 30,
    "city": "New York",
    "isStudent": False,
    "ip": "192.168.76.135",
    "port": 9092,
    "protocol": "kafka",
    "topic": "test",
    "data": {
        "sub_01": "# Initialize Kafka producer - Updated to match Hub configuration",
        "sub_02": "# value_serializer=lambda x: json.dumps(x).encode('utf-8'), 78d9j1mdk1adf_67"
    },
    "partition": 0,
    "offset": 0,
    "courses": [
        "Math",
        "Science"
    ]
}

# Sample data for generating variations
NAMES = ["John Doe", "Jane Smith", "Bob Johnson", "Alice Williams", "Charlie Brown"]
CITIES = ["New York", "Los Angeles", "Chicago", "Houston", "Phoenix"]
IPS = ["192.168.76.135", "10.0.0.1", "172.16.0.1", "192.168.1.100"]
COURSES = ["Math", "Science", "English", "History", "Art", "Music"]


class KafkaProducerPerfTest:
    def __init__(self, brokers, topic, qps=1000, duration=60, num_threads=4, max_messages=None):
        self.brokers = brokers
        self.topic = topic
        self.target_qps = qps
        self.duration = duration
        self.num_threads = num_threads
        self.max_messages = max_messages
        self.producer = None
        self.running = False
        self.stats = {
            "sent": 0,
            "failed": 0,
            "start_time": None,
            "end_time": None
        }
        self.lock = threading.Lock()

    def create_producer(self):
        """Create Kafka producer with optimized settings for high throughput"""
        return KafkaProducer(
            bootstrap_servers=self.brokers,
            value_serializer=lambda x: json.dumps(x).encode('utf-8'),
            # Optimize for throughput
            batch_size=16384,  # 16KB batch size
            linger_ms=10,  # Wait up to 10ms to fill batch
            compression_type='gzip',  # Enable compression
            acks=1,  # Leader acknowledgment only for better throughput
            retries=3,
            max_in_flight_requests_per_connection=5,
            buffer_memory=67108864,  # 64MB buffer
        )

    def generate_message(self, message_id):
        """Generate a test message with variations"""
        msg = DEFAULT_MESSAGE.copy()
        msg["name"] = random.choice(NAMES)
        msg["age"] = random.randint(18, 65)
        msg["city"] = random.choice(CITIES)
        msg["ip"] = random.choice(IPS)
        msg["isStudent"] = random.choice([True, False])
        msg["courses"] = random.sample(COURSES, random.randint(2, 4))
        
        # Add message ID for tracking
        msg["message_id"] = message_id
        msg["timestamp"] = int(time.time() * 1000)
        
        return msg

    def send_messages(self, thread_id):
        """Send messages in a loop for the specified duration"""
        messages_per_thread = self.target_qps // self.num_threads
        interval = 1.0 / messages_per_thread if messages_per_thread > 0 else 0.001
        
        end_time = time.time() + self.duration
        message_id_base = thread_id * 1000000
        
        while self.running and time.time() < end_time:
            # Check max messages limit
            with self.lock:
                if self.max_messages and self.stats["sent"] >= self.max_messages:
                    break
            
            try:
                msg = self.generate_message(message_id_base + self.stats["sent"])
                future = self.producer.send(self.topic, value=msg)
                
                # Non-blocking send, handle callback
                future.add_callback(self._on_send_success)
                future.add_errback(self._on_send_error)
                
                with self.lock:
                    self.stats["sent"] += 1
                    # Check limit again after increment
                    if self.max_messages and self.stats["sent"] >= self.max_messages:
                        self.running = False
                        break
                
                # Control rate
                if interval > 0:
                    time.sleep(interval)
                    
            except Exception as e:
                print(f"Thread {thread_id} error: {e}", file=sys.stderr)
                with self.lock:
                    self.stats["failed"] += 1

    def _on_send_success(self, record_metadata):
        """Callback for successful send"""
        pass

    def _on_send_error(self, exception):
        """Callback for send error"""
        with self.lock:
            self.stats["failed"] += 1
        print(f"Send error: {exception}", file=sys.stderr)

    def run(self):
        """Run the performance test"""
        print(f"Starting Kafka producer performance test")
        print(f"Brokers: {self.brokers}")
        print(f"Topic: {self.topic}")
        print(f"Target QPS: {self.target_qps}")
        print(f"Duration: {self.duration}s")
        print(f"Threads: {self.num_threads}")
        print("-" * 60)

        try:
            self.producer = self.create_producer()
            self.running = True
            self.stats["start_time"] = time.time()

            # Start producer threads
            with ThreadPoolExecutor(max_workers=self.num_threads) as executor:
                futures = []
                for i in range(self.num_threads):
                    future = executor.submit(self.send_messages, i)
                    futures.append(future)

                # Wait for all threads to complete
                for future in futures:
                    future.result()

            # Flush remaining messages
            self.producer.flush()
            self.stats["end_time"] = time.time()

        except KeyboardInterrupt:
            print("\nInterrupted by user")
            self.running = False
        except Exception as e:
            print(f"Error: {e}", file=sys.stderr)
        finally:
            if self.producer:
                self.producer.close()
            self.print_stats()

    def print_stats(self):
        """Print performance statistics"""
        elapsed = self.stats["end_time"] - self.stats["start_time"] if self.stats["end_time"] else 0
        actual_qps = self.stats["sent"] / elapsed if elapsed > 0 else 0
        
        print("\n" + "=" * 60)
        print("Performance Test Results")
        print("=" * 60)
        print(f"Total messages sent: {self.stats['sent']:,}")
        print(f"Failed messages: {self.stats['failed']:,}")
        print(f"Duration: {elapsed:.2f}s")
        print(f"Actual QPS: {actual_qps:,.0f}")
        print(f"Target QPS: {self.target_qps:,}")
        print(f"Success rate: {(1 - self.stats['failed']/max(self.stats['sent'], 1)) * 100:.2f}%")
        print("=" * 60)


def signal_handler(sig, frame):
    """Handle Ctrl+C gracefully"""
    print("\nShutting down...")
    sys.exit(0)


def main():
    parser = argparse.ArgumentParser(
        description="Kafka Producer Performance Test Script",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Basic test: 1000 QPS for 60 seconds
  python3 kafka_producer_perf_test.py --brokers localhost:9092 --topic test

  # High throughput test: 40000 QPS for 300 seconds with 8 threads
  python3 kafka_producer_perf_test.py --brokers 192.168.0.105:9092 --topic test --qps 40000 --duration 300 --threads 8

  # Quick test: 100 QPS for 10 seconds
  python3 kafka_producer_perf_test.py --brokers localhost:9092 --topic test --qps 100 --duration 10
        """
    )
    
    parser.add_argument(
        "--brokers",
        type=str,
        default="localhost:9092",
        help="Kafka broker addresses (comma-separated) (default: localhost:9092)"
    )
    
    parser.add_argument(
        "--topic",
        type=str,
        default="test",
        help="Kafka topic name (default: test)"
    )
    
    parser.add_argument(
        "--qps",
        type=int,
        default=1000,
        help="Target messages per second (default: 1000)"
    )
    
    parser.add_argument(
        "--duration",
        type=int,
        default=60,
        help="Test duration in seconds (default: 60)"
    )
    
    parser.add_argument(
        "--threads",
        type=int,
        default=4,
        help="Number of producer threads (default: 4)"
    )
    
    parser.add_argument(
        "--max-messages",
        type=int,
        default=None,
        help="Maximum number of messages to send (default: unlimited, controlled by duration)"
    )

    args = parser.parse_args()
    
    # Safety check: warn if high QPS or long duration
    if args.qps > 10000:
        print(f"⚠️  WARNING: High QPS ({args.qps:,}) may stress your Kafka cluster!", file=sys.stderr)
        print(f"   Press Ctrl+C to stop at any time.", file=sys.stderr)
    
    if args.duration > 600:
        print(f"⚠️  WARNING: Long duration ({args.duration}s = {args.duration/60:.1f} minutes)!", file=sys.stderr)
        print(f"   Press Ctrl+C to stop at any time.", file=sys.stderr)

    # Handle Ctrl+C
    signal.signal(signal.SIGINT, signal_handler)

    # Parse brokers
    brokers = [b.strip() for b in args.brokers.split(",")]

    # Create and run test
    test = KafkaProducerPerfTest(
        brokers=brokers,
        topic=args.topic,
        qps=args.qps,
        duration=args.duration,
        num_threads=args.threads,
        max_messages=args.max_messages
    )
    
    test.run()


if __name__ == "__main__":
    main()
