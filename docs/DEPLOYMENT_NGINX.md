# Host-Managed Nginx & Reverse Proxy Deployment Guide

Simple-Trader is architected for production VPS hosting where the system administrator manages Nginx directly on the host operating system. This architecture provides:
- Native **Let's Encrypt** SSL/TLS certificate automation via `certbot`
- Direct host-level rate limiting, DDoS mitigation, and firewall integration
- Zero-overhead proxying to the containerized Pure Go backend (`http://127.0.0.1:8080`)
- Native HTTP/1.1 chunked streaming for real-time Server-Sent Events (SSE)

---

## 🏗️ Architecture Overview

```
[ Public Internet / Clients / Mobile PWA ]
                    │
                    ▼  (Port 80 / 443 SSL)
┌─────────────────────────────────────────────────────────────┐
│               Host Nginx Web Server / Proxy                 │
│  - SSL Termination & HTTP/2                                 │
│  - Gzip / Brotli Compression                                │
│  - Unbuffered SSE Passthrough (/api/v1/events)              │
│  - Static Asset Caching                                     │
└─────────────────────────────┬───────────────────────────────┘
                              │ Reverse Proxy (127.0.0.1:8080)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│          Simple-Trader Containerized Go Backend             │
│  - REST API & Real-time Signal Engine                       │
│  - Compiled React 18 SPA Direct File Serving                │
└─────────────────────────────────────────────────────────────┘
```

---

## 📋 Prerequisites

Install Nginx and Certbot on your VPS host (Ubuntu / Debian):

```bash
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx
```

Verify that Nginx is running:
```bash
sudo systemctl status nginx
```

---

## ⚙️ Nginx Configuration

Create a dedicated server configuration in `/etc/nginx/sites-available/simple-trader`:

```bash
sudo nano /etc/nginx/sites-available/simple-trader
```

Paste the following production configuration:

```nginx
upstream simple_trader_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name trading.yourdomain.com; # Replace with your domain name or VPS IP

    # High-Performance Gzip Compression
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_proxied expired no-cache no-store private auth;
    gzip_types text/plain text/css text/xml text/javascript application/javascript application/json image/svg+xml;
    gzip_disable "MSIE [1-6]\.";

    client_max_body_size 20M;

    # 1. System Health Check Endpoint
    location /health {
        proxy_pass http://simple_trader_backend/health;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 2. Real-Time Server-Sent Events (SSE) Stream
    # CRITICAL: Disable buffering and caching for instantaneous streaming
    location /api/v1/events {
        proxy_pass http://simple_trader_backend/api/v1/events;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable proxy buffering for zero-latency trade signal delivery
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }

    # 3. Backend REST API Endpoints
    location /api/ {
        proxy_pass http://simple_trader_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }

    # 4. Progressive Web App (PWA) & Static Assets
    # Directly served by the Go backend with automatic SPA index.html fallback
    location / {
        proxy_pass http://simple_trader_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 🚀 Activation & Validation

1. **Enable the site configuration**:
   ```bash
   sudo ln -s /etc/nginx/sites-available/simple-trader /etc/nginx/sites-enabled/
   ```

2. **Test configuration syntax**:
   ```bash
   sudo nginx -t
   ```

3. **Reload Nginx**:
   ```bash
   sudo systemctl reload nginx
   ```

---

## 🔒 SSL / TLS Certificate with Let's Encrypt

Obtain and configure an automated SSL certificate using `certbot`:

```bash
sudo certbot --nginx -d trading.yourdomain.com
```

Certbot will automatically configure HTTP-to-HTTPS redirection, TLS 1.3 ciphers, and scheduled renewal crons. Test the renewal timer:

```bash
sudo certbot renew --dry-run
```
