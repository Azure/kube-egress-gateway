#!/usr/bin/env bash
#
# Run CNI plugin unit tests.
# 
set -e

# switch into the repo root directory
GIT_ROOT=$(git rev-parse --show-toplevel)
cd $GIT_ROOT

echo "Running tests"

mkdir -p /tmp/cni-rootless
declare -a pkg_need_root=("github.com/Azure/kube-egress-gateway/cmd/kube-egress-cni" "github.com/Azure/kube-egress-gateway/cmd/kube-egress-cni-ipam")

PKG=${PKG:-$(go list ./... | xargs echo)}

for t in ${PKG}; do
    if [[ " ${pkg_need_root[*]}"  == *"${t}"* ]];
    then 
        bash -c "export XDG_RUNTIME_DIR=/tmp/cni-rootless; unshare -rmn go test ${t} -covermode set"
    else
        go test ${t} -covermode set
    fi
done

rm -rf /tmp/cni-rootless
