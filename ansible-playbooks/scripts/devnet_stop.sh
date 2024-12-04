#!/bin/bash

docker container prune -f
docker compose -f /home/moichain/stack/moipod/docker-compose.yml down
