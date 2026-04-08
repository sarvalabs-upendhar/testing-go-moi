#!/bin/bash

docker container prune -f
docker compose -f /home/{{ app_user }}/stack/moipod/docker-compose.yml down
