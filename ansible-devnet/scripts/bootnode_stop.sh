#!/bin/bash

docker container prune -f
docker compose -f /home/ubuntu/stack/bootnode/docker-compose.yml down
