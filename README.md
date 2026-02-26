<h1>
  <img src="https://img.icons8.com/?size=100&id=5lRHx9hqQq5i&format=png&color=000000" width="32" style="vertical-align: middle;" />
  waford
</h1>

![Go Version](https://img.shields.io/badge/Go-1.26.0-00ADD8?style=for-the-badge&logo=go)
![Dependencies](https://img.shields.io/badge/Dependencies-Zero-brightgreen?style=for-the-badge)

**waford** is a zero-dependency, high-throughput webhook fan-out service written in Go. 

It securely receives a single incoming webhook payload and asynchronously distributes it to multiple downstream destinations while natively handling partial failures, system backpressure, and graceful degradation.

## ⚡ Why waford?

This is an internal micro tool designed to solve a specific problem where most webhooks only allow a single endpoint registration. Sometimes you might want to send this webhook event to multiple endpoints, services, apps, users or whatever it is. `waford` allows you to fan out and send a single webhook across multiple destination proposing a neat asynchronous solution.  

📖 **[Read the full architectural deep-dive and build journey here!](https://lennzer.vercel.app/posts/waford_h)**

## ✨ Core Features

* **Micro-Job Fan-Out:** Ingress payloads are instantly split into isolated micro-jobs, ensuring that a slow destination never blocks the delivery of a healthy destination.
* **Exponential Backoff with Full Jitter:** Protects recovering downstream servers from "Thundering Herd" DDoS attacks by mathematically desynchronizing retry attempts.
* **Dead Letter Queue (DLQ):** Jobs that exhaust their retry limits are safely flushed to a thread-safe `.jsonl` file on disk, capturing the exact `last_error` for later debugging.
* **Load Shedding & Backpressure:** Protects its own memory. When internal queues reach capacity, `waford` shifts from blocking connections to instantly shedding load with `HTTP 429 Too Many Requests`.
* **Graceful Context Shutdowns:** Uses `select` statements and `context.Context` to ensure background workers cleanly abort sleeping timers and flush buffers to disk on `SIGTERM`, guaranteeing zero panics and no lost data.

## 🚀 Getting Started

### Prerequisites
* Go 1.26 or higher

### Installation

1. Clone the repository:
```bash
git clone https://github.com/segfaultscribe/waford
cd waford
```
2. Install dependencies
```bash
go mod tidy
```
3. Start the server
```bash   
go run main.go
```

### 💻 Usage

**waford** exposes a single, lightning-fast ingress endpoint. It accepts your payload, drops it into the internal channel buffers, and instantly returns a 202 Accepted to free up the client.

Send a Webhook:
```bash
curl -X POST http://localhost:3000/ingress \
     -H "Content-Type: application/json" \
     -d '{"event": "user.signup", "user_id": "12345" "plan": "pr
```
you can see the logs appear on your console.

`NOTE`: On windows you might want to create a .json file (eg: payload.json) and use it instead.

```powershell
curl -X POST http://localhost:3000/ingress `
-H "Content-Type: application/json" `
--data-binary "@payload.json"
```

## 📊 Benchmarks

**waford** is built to maximize the underlying OS's TCP limits. In local stress tests using hey, waford dynamically balanced background fan-out processing with active load shedding.

A basic test was made with a downstream chaos server. The chaos server was designed to be a probabilistically hostile with:
```
33% of the time: Return 200 OK instantly. (simulate success)
33% of the time: Return 500 Internal Server Error. (simulate failure)
34% of the time: Sleep for 6 seconds and do nothing. (simulate slow downstream)
```

### Load test 1
**Configuration:** Buffer: 100 | Workers: 100 / 10 retry / 1 DLQ<br/>

**Load:** 5,000 concurrent webhooks (15,000 internal jobs) fired in 1 second (100 concurrent connections) using **[hey](https://github.com/rakyll/hey)**.

    Average Ingress Latency: 0.02 seconds

    Success Rate: 100% (Mix of 202 Accepted for buffered jobs and 429 Too Many Requests for successfully shed load to prevent OOM).


### Load test 2
**Configuration:** Buffer: 20,000 | Workers: 1,000 Fresh / 200 Retry / 1 DLQ

**Load:** 20,000 webhooks across 500 concurrent connections.

```
OS Bottleneck Hit: Windows actively blocked ~1,700 requests due to ephemeral port exhaustion (connectex error) before they even reached the code.

waford flawlessly ingested the surviving ~18,200 requests in just 4.9 seconds.

Maintained an average ingress latency of 0.12 seconds under extreme duress.

~4k RPS
```

**NOTE:** Testing on a proper linux server is pending. The above tests were run on a windows system.