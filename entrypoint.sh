#!/bin/bash
set -e

ROLE=$1
BOOTSTRAP_FILE="/shared/bootstrap.addr"

if [ "$ROLE" == "node1" ]; then
    echo "Starting Node 1 (Bootstrap)..."
    ./backend-app --headless --db /data/app.db --p2p-port 4001 --write-bootstrap "$BOOTSTRAP_FILE"
else
    echo "Starting $ROLE..."
    # Wait for bootstrap file if we want to test bootstrap mode
    # Or just start if we want to test mDNS
    if [ "$USE_BOOTSTRAP" == "true" ]; then
        echo "Waiting for bootstrap file..."
        while [ ! -f "$BOOTSTRAP_FILE" ]; do sleep 1; done
        ADDR=$(cat "$BOOTSTRAP_FILE")
        echo "Found bootstrap address: $ADDR"
        ./backend-app --headless --db /data/app.db --bootstrap "$ADDR"
    else
        echo "Starting in pure mDNS mode..."
        ./backend-app --headless --db /data/app.db
    fi
fi
