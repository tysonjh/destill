# Docker Infrastructure

Docker Compose configuration for Destill's distributed mode.

## Services

| Service | Port | Purpose |
|---------|------|---------|
| Redpanda | 19092 | Message broker (Kafka API) |
| Postgres | 5432 | Persistent storage |
| Redpanda Connect | 4195 | Kafka-to-Postgres sink |
| Redpanda Console | 8080 | Web UI for monitoring |

## Quick start

```bash
docker-compose up -d
docker-compose ps  # verify health
```

## Create topics

```bash
docker exec -it destill-redpanda rpk topic create destill.logs.raw --partitions 3
docker exec -it destill-redpanda rpk topic create destill.analysis.findings --partitions 3
docker exec -it destill-redpanda rpk topic create destill.requests --partitions 1
```

## Environment variables

```bash
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"
```

## Verify setup

```bash
# Redpanda
docker exec -it destill-redpanda rpk cluster health

# Postgres
docker exec -it destill-postgres psql -U destill -d destill -c '\dt'

# Console
open http://localhost:8080
```

## Cleanup

```bash
docker-compose down     # stop services
docker-compose down -v  # stop and remove data
```
