#!/usr/bin/env bash
# Nightly database dump kept for 14 days (installed as a cron job by the server setup).
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p backups
docker compose exec -T db pg_dump -U crm -d crm --format=custom > "backups/crm-$(date +%F).dump"
find backups -name 'crm-*.dump' -mtime +14 -delete
