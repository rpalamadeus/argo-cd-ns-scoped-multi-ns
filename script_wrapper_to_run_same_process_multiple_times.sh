#!/bin/bash

# Array of commands to run as child processes
commands=(
    "while true; do echo 'Process 1'; sleep 2; done"
    "while true; do echo 'Process 2'; sleep 3; done"
    "sleep 2 ; bash -c 'exit 1'"
    # here we can paas on other commands and arguments required to run spawn up multiple process
)

# Associative array to keep track of process IDs
declare -A pids

# Function to start a process
start_process() {
    local cmd="$1"
    echo "[master]Starting process: $cmd"
    bash -c "$cmd" 2>&1 | while IFS= read -r line; do
        echo "[master][abcd] $line"
    done &
    local pid=$!
    pids[$pid]="$cmd"
}

# Function to monitor each process in a separate thread
monitor_process() {
    local pid=$1
    while kill -0 "$pid" 2>/dev/null; do
        sleep 1
    done
    wait $pid
    status=$?
    if [ $status -ne 0 ]; then
        echo "[master]Process $pid exited with error status $status."
    else
        echo "[master]Process $pid terminated gracefully."
    fi
    unset pids[$pid]
}

# Graceful shutdown of child processes
graceful_shutdown() {
    echo "[master]Shutting down all child processes..."
    for pid in "${!pids[@]}"; do
        echo "[master]Terminating process $pid"
        kill "$pid"
        wait "$pid"
    done
    exit 0
}

trap graceful_shutdown SIGINT SIGTERM

# Start all initial processes
for cmd in "${commands[@]}"; do
    start_process "$cmd"
    pid=$!
    monitor_process "$pid" &
done
