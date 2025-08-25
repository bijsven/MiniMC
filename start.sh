#!/bin/sh
MINECRAFT_SUBDOMAIN="minimc-mc"
WEB_SUBDOMAIN="minimc-web"

# Start Minecraft tunnel (TCP 25565)
ssh -o StrictHostKeyChecking=no -R ${MINECRAFT_SUBDOMAIN}:25565:localhost:25565 serveo.net -f -N

# Start Web UI tunnel (TCP 8080)
ssh -o StrictHostKeyChecking=no -R ${WEB_SUBDOMAIN}:80:localhost:8080 serveo.net -f -N

echo "[i] Minecraft is available on: ${MINECRAFT_SUBDOMAIN}.serveo.net:25565"
echo "[i] Web UI is available on: ${WEB_SUBDOMAIN}.serveo.net"

./MiniMC
