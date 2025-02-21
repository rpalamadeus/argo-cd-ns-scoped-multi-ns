#!/bin/bash
# below command is to find list of all modified files and to find diff of all these files across two different versions

if [ -f ./list_of_modified_files.log ]; then
    rm list_of_modified_files.log
fi

find . -type f -path "./cmd/*.go" | while read -r file; do
    echo $file>>list_of_modified_files.log
done

cd argo-cd
cat ../list_of_modified_files.log|while read -r file; do
    echo "=========================================================="
    echo $file
    echo "=========================================================="
    git diff --color-words $(git log -1 --pretty=%s) $(cat ../VERSION_ARGOCD) -- $file
done
cd ..
