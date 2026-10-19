#!/usr/bin/env bash
# Regenerate the README screenshots.
# Requires Google Chrome (or CHROME_PATH) and free TCP 8080 and UDP 69.
# Usage, from the repository root: bash scripts/capture.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

npm install --prefix ./scripts --silent
if [[ ! -x public/node_modules/.bin/tailwindcss ]]; then
  npm install --prefix ./public --silent
fi
./public/node_modules/.bin/tailwindcss -i ./public/http/css/includes.css -o ./public/http/css/tailwind.css --minify
go build -o bin/ignite .

dbdir="$(mktemp -d /tmp/ignite-screenshots.XXXXXX)"
export DB_PATH="$dbdir" DB_FILE="ignite.db"
go run ./scripts/seed-demo.go

./bin/ignite >"$dbdir/server.log" 2>&1 &
pid=$!
cleanup() {
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 40); do
  if curl -sf -o /dev/null "http://127.0.0.1:8080/login"; then
    ready=1
    break
  fi
  sleep 0.3
done
if [[ "$ready" -ne 1 ]]; then
  echo "Ignite did not start. Log:" >&2
  cat "$dbdir/server.log" >&2
  exit 1
fi

node ./scripts/screenshots.mjs
rm -f public/http/img/status_page.png
echo "screenshots written to public/http/img/"
