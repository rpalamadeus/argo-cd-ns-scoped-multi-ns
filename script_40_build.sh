#!/bin/bash
# Run image build within argo-cd folder
cd argo-cd
if which docker > /dev/null 2>&1; then
    docker build -t argo-cd-ns-scoped-multi-ns . --tls-verify=false
elif which podman > /dev/null 2>&1; then
    podman build -t argo-cd-ns-scoped-multi-ns . --tls-verify=false
fi
cd ..