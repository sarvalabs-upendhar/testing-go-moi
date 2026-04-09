#!/bin/bash

docker compose -f /home/ubuntu/stack/bootnode/docker-compose.yml pull
docker compose -f /home/ubuntu/stack/bootnode/docker-compose.yml up -d
