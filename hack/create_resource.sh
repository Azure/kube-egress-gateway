#/bin/bash

if [ "$1" != "webhook" ] && [ "$1" != "api" ]; then
    echo "Unrecognized first argument. Must be <api/webhook>, \"$1\" provided."
    exit 1
fi

workdir=$(pwd)
echo "workdir: $workdir"

make kubebuilder
cp cmd/kube-egress-gateway-controller/cmd/root.go cmd/kube-egress-gateway-controller/cmd/root.go.bak
cp cmd/kube-egress-gateway-controller/cmd/root.go.bak main.go
./bin/kubebuilder create $@
if [ $? -ne 0 ]; then
    echo "kubebuilder create $1 failed"
    rm -f cmd/kube-egress-gateway-controller/cmd/root.go.bak main.go
    exit 1
fi

diff --brief <(cat cmd/kube-egress-gateway-controller/cmd/root.go.bak) <(sort main.go) >/dev/null
comp_value=$?

if [ $comp_value -eq 1 ]
then
    mv main.go cmd/kube-egress-gateway-controller/cmd/root.go
fi

rm -f cmd/kube-egress-gateway-controller/cmd/root.go.bak main.go

make manifests build
