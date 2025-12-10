# Monitoring Redpanda Connect

Redpanda Connect (formerly Benthos) has its own HTTP API for monitoring, separate from Kafka Connect.

## HTTP Endpoints

Redpanda Connect exposes these endpoints on port 4195:

### Health Check
```bash
curl http://localhost:4195/ready
```

Response:
```json
{
  "statuses": [
    {"label": "", "path": "input", "connected": true},
    {"label": "", "path": "output", "connected": true}
  ]
}
```

### Metrics (Prometheus format)
```bash
curl http://localhost:4195/stats
```

Returns Prometheus-style metrics including:
- `input_received` - Messages received from Kafka
- `output_sent` - Messages sent to Postgres
- `input_latency_ns` - Input processing latency
- `output_latency_ns` - Output processing latency
- `output_error` - Output errors (Postgres insert failures)

### Ping
```bash
curl http://localhost:4195/ping
```

Returns: `pong`

## Monitoring with Prometheus & Grafana

For production monitoring, you can add Prometheus and Grafana to the stack:

### 1. Add to docker-compose.yml

```yaml
  prometheus:
    image: prom/prometheus:latest
    container_name: destill-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
    depends_on:
      - redpanda-connect

  grafana:
    image: grafana/grafana:latest
    container_name: destill-grafana
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    depends_on:
      - prometheus

volumes:
  prometheus-data:
  grafana-data:
```

### 2. Create prometheus.yml

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'redpanda-connect'
    static_configs:
      - targets: ['redpanda-connect:4195']
```

### 3. Access Dashboards

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin)

## Simple Monitoring Script

For quick checks without full Prometheus setup:

```bash
#!/bin/bash
# monitor-connect.sh

watch -n 5 'curl -s http://localhost:4195/stats | \
  grep -E "(input_received|output_sent|output_error)" | \
  grep -v "# "'
```

Run with:
```bash
chmod +x monitor-connect.sh
./monitor-connect.sh
```

## Key Metrics to Watch

### Input Metrics
- `input_received` - Total messages consumed from Kafka
- `input_connection_up` - Connection status to Kafka
- `input_latency_ns` - Time to read from Kafka

### Output Metrics
- `output_sent` - Total messages written to Postgres
- `output_error` - Failed Postgres writes
- `output_latency_ns` - Time to write to Postgres

### Batch Metrics
- `batch_created` - Batches created (should match batching config)
- `batch_sent` - Batches successfully sent

## Checking Logs

View Redpanda Connect logs:
```bash
docker logs destill-connect -f
```

Look for:
- ✅ `"kafka consumer up"` - Successfully connected to Kafka
- ✅ `"sql output connection up"` - Successfully connected to Postgres
- ⚠️ `"error"` - Any errors in processing
- 📊 `"batch processed"` - Batch completion messages

## Troubleshooting

### No metrics showing up
```bash
# Check if Connect is running
docker ps | grep destill-connect

# Check if port is accessible
curl http://localhost:4195/ready
```

### High output_error count
```bash
# Check Connect logs
docker logs destill-connect --tail 50

# Check Postgres connectivity
docker exec -it destill-postgres psql -U destill -d destill -c "SELECT COUNT(*) FROM findings;"
```

### Input lag (not consuming)
```bash
# Check consumer group in Redpanda Console
# http://localhost:8080 -> Consumer Groups -> destill-postgres-sink

# Or use rpk
docker exec -it destill-redpanda rpk group describe destill-postgres-sink
```

## Alert Rules (for Prometheus)

```yaml
groups:
  - name: redpanda_connect
    interval: 30s
    rules:
      - alert: ConnectOutputErrors
        expr: rate(output_error[5m]) > 0
        annotations:
          summary: "Redpanda Connect is failing to write to Postgres"
      
      - alert: ConnectInputDisconnected
        expr: input_connection_up < 1
        annotations:
          summary: "Redpanda Connect lost connection to Kafka"
      
      - alert: ConnectOutputDisconnected
        expr: output_connection_up < 1
        annotations:
          summary: "Redpanda Connect lost connection to Postgres"
```

## Quick Health Check

Single command to verify everything is working:

```bash
echo "Redpanda Connect Health:"
curl -s http://localhost:4195/ready | jq
echo ""
echo "Message Counts:"
curl -s http://localhost:4195/stats | grep -E "^(input_received|output_sent)" | grep -v "#"
echo ""
echo "Errors:"
curl -s http://localhost:4195/stats | grep "error" | grep -v "# " | grep -v " 0$"
```

## Why Not in Redpanda Console?

**Important Note**: Redpanda Console's "Connect" tab is for monitoring **Kafka Connect** (a different product), not **Redpanda Connect** (formerly Benthos). 

- **Kafka Connect**: Java-based, REST API, plugin architecture
- **Redpanda Connect**: Go-based, YAML config, Benthos API

They are incompatible systems despite similar names. Use the HTTP endpoints above for monitoring Redpanda Connect.

