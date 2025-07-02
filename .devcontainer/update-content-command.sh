#!/bin/bash
echo Hello World

echo 'alias k=kubectl' >> ~/.bashrc
echo 'alias tf=terraform' >> ~/.bashrc
echo 'alias uuid="cat /proc/sys/kernel/random/uuid"' >> ~/.bashrc
echo "source <(kubectl completion bash)" >> ~/.bashrc