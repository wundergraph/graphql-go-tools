#!/usr/bin/env bash
set -euo pipefail

echo "Composing caching test subgraphs"

npx -y wgc@latest router compose -i graph.yaml -o config.json

echo "Formatting config"
jq . config.json > config.json.tmp
mv config.json.tmp config.json
