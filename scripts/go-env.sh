#!/bin/sh
# Source from scripts only: keep writable build caches inside this repository.
export GOCACHE="${GOCACHE:-$PROJECT_ROOT/.gocache}"
# Reuse an installed module cache if present; go commands pin versions via go.sum.
