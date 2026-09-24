# Tasks: VPS Production Deployment

1. [x] T1 — Push latest main; confirm CI green and GHCR arm64 image published
2. [x] T2 — Provision /opt/simple-trader on VPS: clone repo (or minimal copy), create production .env from .env.example with real secrets
3. [x] T3 — docker compose pull && docker compose up -d on VPS (prebuilt arm64 images)
4. [x] T4 — Configure host Nginx: SSL termination for nl-main.z3df1lter.uk, proxy to 127.0.0.1:8080, SSE buffering off
5. [x] T5 — Verify: health, assets (123), positions, signals, screener from public HTTPS URL
6. [x] T6 — Harsh post-deploy review: logs, resource usage, restart resilience (docker compose restart; verify rehydration log line)
