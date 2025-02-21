#!/bin/bash
# Initialize and update submodules
git submodule update --init --recursive

# Checkout required argocd version
cd argo-cd
git fetch --tags
git checkout $(cat ../VERSION_ARGOCD)
cd ..

# Disable and copy go.mod to root outside argo-cd
if [ -f ./argo-cd/go.mod ]; then
    cp argo-cd/go.mod .
    mv argo-cd/go.mod argo-cd/go.mod1
    echo "go.mod exists"
fi

# Change package to ns-scoped-multi-ns
if [ "$OSTYPE" = "darwin"* ]; then
    sed -i '' "s/module github.com\/argoproj\/argo-cd\/v2/module ns-scoped-multi-ns/g" go.mod
    echo "replaced module name in mac-os"
else
    sed -i "s/module github.com\/argoproj\/argo-cd\/v2/module ns-scoped-multi-ns/g" go.mod
    echo "replaced module name in unix or windows"
fi
head -10 go.mod

# Run dependency check
go mod tidy