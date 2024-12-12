#!/bin/bash

docker compose -f /home/moichain/stack/bootnode/docker-compose.yml pull
docker compose -f /home/moichain/stack/bootnode/docker-compose.yml up -d
