# URL Shortener

A multi-language URL shortener service built with **Go**, **Python**, and **TypeScript**.

## Architecture

| Component | Language | Description |
|-----------|----------|-------------|
| `api-server/` | Go | HTTP API server for shortening URLs and redirecting |
| `analytics/` | Python | Analytics service for generating reports and statistics |
| `cli-client/` | TypeScript | CLI client for interacting with the API |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/api/shorten` | Shorten a URL |
| GET | `/r/{code}` | Redirect to original URL |
| GET | `/api/stats` | Get all statistics |
| GET | `/api/stats/{code}` | Get stats for a specific URL |

## Quick Start

### Using Docker Compose

```bash
docker compose up --build
```

### Running Individually

**API Server (Go):**
```bash
cd api-server
go run main.go
```

**Analytics (Python):**
```bash
cd analytics
pip install -r requirements.txt
python main.py report --api-url http://localhost:8080
```

**CLI Client (TypeScript):**
```bash
cd cli-client
npm install && npm run build
node dist/index.js shorten https://example.com
```

## Testing

```bash
# Go tests
cd api-server && go test -v ./...

# Python tests
cd analytics && python -m pytest test_analyzer.py -v

# TypeScript tests
cd cli-client && npm install && npm run build && npm test
```

## Usage Examples

```bash
# Shorten a URL
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# Get statistics
curl http://localhost:8080/api/stats

# Use the CLI
node dist/index.js shorten https://example.com
node dist/index.js stats
node dist/index.js lookup <code>
```
