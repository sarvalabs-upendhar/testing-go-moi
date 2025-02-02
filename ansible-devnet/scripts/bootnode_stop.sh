#!/bin/bash

docker container prune -f
docker compose -f /home/{{ app_user }}/stack/bootnode/docker-compose.yml down
