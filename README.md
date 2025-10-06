# Domain Risk Evaluation

Domain Risk Evaluation is a full-stack web application that ingests USPTO trademark bulk XML and a domain catalog to automate trademark and vice-risk scoring. It aligns with the BRD scoring matrix and outputs actionable recommendations for each domain.

## Features

- Streaming USPTO XML ingestion (no full-file load) directly into SQLite.
- Offline trademark risk scoring with fanciful detection, minor variation handling, and compound heuristics.
- Vice domain detection with configurable severity lists.
- REST API powered by Gin with CSV/JSON exports and pagination/search.
- React + Vite + Tailwind front-end for uploads, evaluation execution, and result exploration.
- Dockerized Go (backend) and Node (frontend) services plus Makefile shortcuts.
- Unit tests covering scoring logic.

## Project Structure

```
domain-risk-eval/
  backend/                Go services, scoring, API
  frontend/               React + Tailwind UI
  Dockerfile.backend
  Dockerfile.frontend
  docker-compose.yml
  Makefile
  README.md
```

## Prerequisites

- Go 1.22+
- Node.js 20+
- npm 10+
- (Optional) Docker & Docker Compose v2+

## Quick Start

```bash
# start both services in containers
make dev
# or run locally
cd backend
GOCACHE=$(pwd)/.gocache go run ./cmd/server

# in a separate terminal
cd frontend
npm install
npm run dev
```

Backend defaults:
- Listens on `:2000`
- Looks for `../apc250917.xml` and `../Test domains.csv` relative to the backend directory if uploads are skipped.

Frontend defaults:
- Dev server on `:1000`
- Expects `VITE_API_BASE` (defaults to `http://localhost:2000/api`).

## API Overview

- `POST /api/upload` – accepts multipart (`xml`, `domains`); updates SQLite with marks/domains.
- `POST /api/evaluate` – runs trademark + vice scoring, persists evaluations, returns the first page of results.
- `GET /api/results` – query parameters: `q`, `minScore`, `page`, `pageSize`.
- `GET /api/export.csv` / `GET /api/export.json` – full dataset exports.
- `GET /api/config` – exposes active config.
- `GET /api/healthz` – liveness check.

## Popular Trademark Pipeline

The `popular` CLI ingests USPTO bulk data, aggregates the 500k most common marks, and primes the scoring engine so exact matches to famous brands automatically trigger a review.

```bash
# Example: download all trtyrap files from 1950 onward and build the top 500k list
export USPTO_DATASET_KEY="<dataset api key>"
cd backend
go run ./cmd/popular \
  --from 1950-01-01 \
  --to 2025-12-31 \
  --limit 500000 \
  --min-count 2 \
  --output ../popular-tokens.json

# To reuse existing marks and just refresh aggregates
go run ./cmd/popular --refresh --limit 500000 --min-count 2

# To process locally downloaded ZIPs inside a directory
go run ./cmd/popular \
  --xml-dir "/path/to/uspto-data" \
  --limit 500000 \
  --min-count 2 \
  --output ../popular-tokens.json
```

Important environment variables:

- `USPTO_DATASET_URL` – defaults to `https://api.uspto.gov/api/v1/datasets/products/trtyrap`.
- `USPTO_DATASET_KEY` – required dataset API key.
- `USPTO_DATASET_FROM` / `USPTO_DATASET_TO` – default date range for downloads.
- `POPULAR_MARK_LIMIT` – number of popular tokens the API loads on startup (default `500000`).
- `POPULAR_MARK_MIN_COUNT` – minimum occurrences to treat a mark as popular (default `2`).
- `DISABLE_AI` – set to `true` to skip AI explanations (heuristics only).

> **Upcoming:** the next iteration will stream the 500k popular marks through the AI explainer, store descriptive metadata, and push embeddings into PGVector so semantic trademark lookups can run directly from the database.

## Testing

```bash
cd backend
GOCACHE=$(pwd)/.gocache go test ./...
```

> **Note:** The Go build/test pipeline requires module downloads. If the current environment restricts outbound network calls, run `go mod tidy` once with network access to populate `go.sum` and populate the module cache.

## Environment Variables

- `PORT` – backend port (default `2000`).
- `USPTO_API_KEY` – required for live USPTO trademark lookups.
- `USPTO_BASE_URL` – optional override for the USPTO endpoint (defaults to IBD API publications).
- `USPTO_TIMEOUT` / `USPTO_CACHE_TTL` / `USPTO_ROWS` – optional tuning knobs for USPTO client (duration strings like `20s`, `12h`).
- `VITE_API_BASE` – frontend API base URL.

## Docker

```bash
# Build images
make build

# Launch services
make dev
```

The backend container mounts seeds, vice term configs, and sample XML/CSV files for instant evaluation.

## Future Enhancements

- Plug in live USPTO TSDR API via `internal/tsdr` once credentials are available.
- Expand vice detection with Unicode/homoglyph analysis.
- Persist evaluation batches and support resumable processing for >1M domains.
