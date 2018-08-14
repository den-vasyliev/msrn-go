#!/bin/bash
while ! ping -c 1 redis-0.redis; do
echo 'Waiting for server'
sleep 1
done
cp /redis-config/sentinel.conf /redis-config-rw/sentinel.conf
redis-sentinel /redis-config-rw/sentinel.conf