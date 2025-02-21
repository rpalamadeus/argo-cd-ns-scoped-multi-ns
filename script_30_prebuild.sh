#!/bin/bash
# Copy modified files and folders to argo-cd
cp -rf cmd/* argo-cd/cmd/.

# Enable go build within argo-cd directory
if [ -f argo-cd/go.mod1 ]; then
    mv argo-cd/go.mod1 argo-cd/go.mod
fi
