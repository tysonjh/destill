# Distributed Mode Quick Start

## 1. Build

```bash
make build
```

## 2. Start infrastructure

```bash
cd docker && docker-compose up -d
```

## 3. Create topics

```bash
docker exec -it destill-redpanda rpk topic create destill.logs.raw --partitions 3
docker exec -it destill-redpanda rpk topic create destill.analysis.findings --partitions 3
docker exec -it destill-redpanda rpk topic create destill.requests --partitions 1
```

## 4. Set environment

```bash
export BUILDKITE_API_TOKEN="your_token"
export GITHUB_TOKEN="your_token"
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"
```

## 5. Start agents

In separate terminals:

```bash
./bin/destill-ingest
./bin/destill-analyze
```

## 6. Submit and view

```bash
./bin/destill submit "https://buildkite.com/org/pipeline/builds/123"
# Returns: req-20251215T143022-a1b2c3d4

./bin/destill view req-20251215T143022-a1b2c3d4
```

## Monitoring

- Redpanda Console: http://localhost:8080
- Postgres: `docker exec -it destill-postgres psql -U destill -d destill`

## Cleanup

```bash
cd docker && docker-compose down -v
```
