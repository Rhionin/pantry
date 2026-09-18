#!/bin/sh
docker rm -f kiro-sandbox
docker run -d \
  --name kiro-sandbox \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  -v "$(pwd)":/workspace \
  -w /workspace \
  kiro-sandbox tail -f /dev/null

