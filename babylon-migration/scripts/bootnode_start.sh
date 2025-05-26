#!/bin/bash

docker compose -f /home/{{ app_user }}/stack/bootnode/docker-compose.yml pull
docker compose -f /home/{{ app_user }}/stack/bootnode/docker-compose.yml up -d
