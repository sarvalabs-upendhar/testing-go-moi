#!/bin/bash

docker container prune -f
docker compose -f /home/moichain/stack/bootnode/docker-compose.yaml down
