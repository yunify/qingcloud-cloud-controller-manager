#!/bin/bash

#build binary

set -e
tag=`git rev-parse --short HEAD`
IMG=magicsong/cloud-manager:$tag
DEST=test/manager.yaml
echo "Building binary"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-w" -o bin/manager ./cmd/main.go

echo "Building docker image"
docker build -t $IMG  -f deploy/Dockerfile bin/
echo "Push images"
docker push $IMG
echo "Generating yaml"

sed -e 's@image: .*@image: '"${IMG}"'@' deploy/kube-cloud-controller-manager.yaml > $DEST

kubectl apply -f $DEST