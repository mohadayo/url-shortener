# Pull Request: feat/url-shortener-service -> main

## Title
feat: add multi-language URL shortener service

## Summary
- Go API server with URL shortening, redirect, and statistics endpoints (in-memory store)
- Python analytics service with domain extraction, click tracking, and report generation
- TypeScript CLI client for shortening URLs, viewing stats, and health checks
- Docker Compose setup for running all services together

## Test Plan
- [x] Go unit tests pass (5/5) - `cd api-server && go test -v ./...`
- [x] Python unit tests pass (8/8) - `cd analytics && python -m unittest test_analyzer -v`
- [x] TypeScript build succeeds and tests pass (3/3) - `cd cli-client && npm run build && npm test`
- [ ] Integration test: `docker compose up --build` and verify end-to-end flow

## Files Changed
- 20 files changed, 990 insertions(+)

## Branch
`feature/url-shortener-service` -> `main`
