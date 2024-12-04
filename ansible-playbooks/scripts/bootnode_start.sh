#!/bin/bash
sleep 10
docker compose -f /home/moichain/stack/bootnode/docker-compose.yaml pull
docker compose -f /home/moichain/stack/bootnode/docker-compose.yaml up -d
