#!/usr/bin/env bash
# Builds the image locally and ships it to the server over SSH; no registry needed.
# Usage: deploy/deploy.sh ubuntu@1.2.3.4 [ssh key]   (PLATFORM defaults to linux/amd64)
set -euo pipefail
host=${1:?usage: deploy/deploy.sh user@host [ssh-key]}
key=${2:-}
ssh_opts=(-o StrictHostKeyChecking=accept-new)
[[ -n $key ]] && ssh_opts+=(-i "$key")
cd "$(dirname "$0")/.."

docker buildx build --platform "${PLATFORM:-linux/amd64}" -t vodafone-crm:latest --load .
docker save vodafone-crm:latest | gzip | ssh "${ssh_opts[@]}" "$host" 'gunzip | sudo docker load'
scp "${ssh_opts[@]}" deploy/compose.yml deploy/Caddyfile deploy/backup.sh "$host":~/crm/
ssh "${ssh_opts[@]}" "$host" 'cd ~/crm && sudo docker compose up -d --remove-orphans && sudo docker image prune -f >/dev/null && sudo docker compose ps'
