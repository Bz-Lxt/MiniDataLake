#!/bin/sh
set -eu
BASE=${1:-http://127.0.0.1:18420}
echo "== health =="
wget -qO- "$BASE/api/v1/health"
echo
echo "== catalog =="
wget -qO- "$BASE/api/v1/catalog"
echo
echo "== stats =="
wget -qO- "$BASE/api/v1/system/stats" | head -c 400
echo
echo "SMOKE_OK"
