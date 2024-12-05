#!/bin/bash
sleep 10
docker compose -f /home/moichain/stack/bootnode/docker-compose.yml pull
docker compose -f /home/moichain/stack/bootnode/docker-compose.yml up -d
