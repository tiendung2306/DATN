#!/bin/bash
set -e

# Forward all arguments to backend-app
exec ./backend-app "$@"
