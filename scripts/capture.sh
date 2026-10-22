#!/usr/bin/env bash
# Regenerate the README screenshots from the binary this script just built.
# Requires Google Chrome (or CHROME_PATH) and free TCP 8080 and UDP 69.
# Page templates are html/template files read from ./templates when the
# process starts; there is no separate template compiler. CSS and images are
# embedded by go:embed, so the binary is rebuilt after Tailwind runs.
# Usage, from the repository root: bash scripts/capture.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# Print socket inodes listening on a TCP or UDP port.
listening_inodes() {
  python3 - "$1" "$2" <<'PY'
import sys
port, proto = int(sys.argv[1]), sys.argv[2]
files = ["/proc/net/tcp", "/proc/net/tcp6"] if proto == "tcp" else ["/proc/net/udp", "/proc/net/udp6"]
want = f"{port:04X}"
found = []
for path in files:
    try:
        lines = open(path).read().splitlines()[1:]
    except FileNotFoundError:
        continue
    for line in lines:
        parts = line.split()
        local, state, inode = parts[1], parts[3], parts[9]
        if local.rsplit(":", 1)[-1].upper() != want:
            continue
        if proto == "tcp" and state != "0A":
            continue
        found.append(inode)
print(" ".join(found))
PY
}

# Print pids that own those socket inodes.
pids_for_inodes() {
  local inode pid fd target
  for inode in "$@"; do
    [[ -n "$inode" ]] || continue
    for fd in /proc/[0-9]*/fd/*; do
      target="$(readlink "$fd" 2>/dev/null || true)"
      if [[ "$target" == "socket:[$inode]" ]]; then
        pid="${fd#/proc/}"
        pid="${pid%%/*}"
        echo "$pid"
      fi
    done
  done
}

require_port_free() {
  local proto="$1" port="$2"
  local inodes pids
  inodes="$(listening_inodes "$port" "$proto")"
  if [[ -z "${inodes// }" ]]; then
    return 0
  fi
  pids="$(pids_for_inodes $inodes | sort -u | tr '\n' ' ')"
  echo "Refusing to capture: ${proto} ${port} is already in use (pid ${pids:-unknown})." >&2
  echo "A leftover server would answer /login and the screenshots would be stale." >&2
  exit 1
}

npm install --prefix ./scripts --silent
if [[ ! -x public/node_modules/.bin/tailwindcss ]]; then
  npm install --prefix ./public --silent
fi
./public/node_modules/.bin/tailwindcss -i ./public/http/css/includes.css -o ./public/http/css/tailwind.css --minify
rm -f bin/ignite
go build -o bin/ignite .

require_port_free tcp 8080
require_port_free udp 69

dbdir="$(mktemp -d /tmp/ignite-screenshots.XXXXXX)"
# The parent shell may still export a previous lab's TFTP_DIR, PROV_DIR, or
# DHCP_ENABLED. Screenshots have to come from this repo's defaults.
unset TFTP_DIR PROV_DIR DHCP_ENABLED SYSLINUX_CACHE_DIR HTTP_PORT HTTP_DIR
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
  if ! kill -0 "$pid" 2>/dev/null; then
    break
  fi
  sleep 0.3
done
if [[ "$ready" -ne 1 ]]; then
  echo "Ignite did not start. Log:" >&2
  cat "$dbdir/server.log" >&2
  exit 1
fi

listener="$(pids_for_inodes $(listening_inodes 8080 tcp) | sort -u | tr '\n' ' ')"
if [[ "$listener" != *"$pid"* ]]; then
  echo "Port 8080 is served by pid [${listener}] instead of the binary just started ($pid)." >&2
  exit 1
fi

curl -fsS "http://127.0.0.1:8080/public/http/css/tailwind.css" -o "$dbdir/served.css"
curl -fsS "http://127.0.0.1:8080/public/http/img/flame-nav.png" -o "$dbdir/served-flame.png"
cmp -s public/http/css/tailwind.css "$dbdir/served.css" || {
  echo "The server is serving CSS that is not the Tailwind file just built." >&2
  exit 1
}
cmp -s public/http/img/flame-nav.png "$dbdir/served-flame.png" || {
  echo "The server is serving a flame-nav.png that is not the current asset." >&2
  exit 1
}
login_html="$(curl -fsS http://127.0.0.1:8080/login)"
if ! grep -q 'flame-nav@2x.png' <<<"$login_html"; then
  echo "Login HTML does not reference the current flame asset." >&2
  exit 1
fi
echo "Serving pid $pid from $(readlink -f bin/ignite); embedded CSS and flame match this tree."

node ./scripts/screenshots.mjs
rm -f public/http/img/status_page.png
echo "screenshots written to public/http/img/"
