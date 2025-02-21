#!/bin/bash
VERSION_FROM_PREVIOUS_COMMIT=$(git log -1 --pretty=%s)
VERSION_ARGOCD=$(cat VERSION_ARGOCD)
if [ $VERSION_ARGOCD = $VERSION_FROM_PREVIOUS_COMMIT ]; then
    git commit --amend --no-edit && git push -f
else 
    git commit -m "$VERSION_ARGOCD" && git push -f
fi