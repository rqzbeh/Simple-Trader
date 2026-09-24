# Feature: VPS Production Deployment

**Branch/PR naming**: `deploy/vps-nl-main`
**Target host**: nl-main.z3df1lter.uk (root@, port 22, key `/home/redsnow/ssh-key/ssh-key-2026-08-30.key`)

## Overview
Deploy the Simple-Trader containerized stack (PostgreSQL 16 + Redis 7 + Go backend serving the compiled PWA) on the user's ARM64 VPS behind Nginx, following GitHub Spec Kit discipline.

## Requirements

### R1: Host preparation
- SSH reachability verified (done: hostname nl-main, aarch64, Ubuntu 26.04.1, Docker 29.8.0, Compose v5.5.1, 23Gi RAM, 79G free disk).
- Application directory `/opt/simple-trader` with env file holding production secrets (not the dev .env).

### R2: Image deployment
- Images are published multi-arch (linux/amd64 + linux/arm64) to GHCR by CI on green main builds (`ghcr.io/rqzbeh/simple-trader-backend:latest`).
- Deploy MUST pull the prebuilt arm64 image; no compilation on the VPS.
- Postgres and Redis via official images (arm64).

### R3: Configuration
- `.env` on the VPS carries: DATABASE_URL, REDIS_URL, AI gateway credentials, ADMIN_PASSWORD, APP_SECRET, all risk bounds.
- Every value read from env at boot — zero hardcoded risk parameters in code (already enforced in codebase).

### R4: Networking
- Backend binds 8080 in-container; Nginx on host terminates SSL for nl-main.z3df1lter.uk and proxies to 127.0.0.1:8080.
- SSE route must disable proxy_buffering (spec: `/api/v1/events`).
- Proxy usage from dev machine only if needed: 127.0.0.1:10808.

### R5: Health & verification
- Post-deploy: `/health` returns healthy; `/api/v1/assets` returns the full live catalog; positions and signals endpoints respond; screener reports qualified universe.
- Fail-fast behavior: dead DB degrades to in-memory mode, never hangs.

## Success Criteria
1. `docker compose ps` shows 3 services running on the VPS.
2. Health endpoint healthy from public URL over HTTPS.
3. Assets endpoint returns 123 instruments with live prices.
4. No secrets in Git history or repo (dev .env never deployed).
