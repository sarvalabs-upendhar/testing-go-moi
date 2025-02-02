#!/bin/bash
sleep 10
docker compose -f /home/{{ app_user }}/stack/moipod/docker-compose.yml pull
docker compose -f /home/{{ app_user }}/stack/moipod/docker-compose.yml up -d
