#!/bin/bash

set -e


SKIP_BUILD=no
tag=`git rev-parse --short HEAD`
IMG=magicsong/cloud-manager:$tag
DEST=test/manager.yaml
TEST_NS=cloud-test-$tag
secret_file=${HOME}/.qingcloud/config.yaml
#build binary

function cleanup(){
    result=$?
    set +e
    echo "Cleaning Namespace"
    kubectl delete secret qcsecret -n $TEST_NS
    kubectl delete ns $TEST_NS > /dev/null
    rm -f $secret_file
    if [ $SKIP_BUILD == "no" ]; then
        docker  rmi $IMG
    fi
    exit $result
}
set -e
trap cleanup EXIT SIGINT SIGQUIT

while [[ $# -gt 0 ]]
do
key="$1"

case $key in
    -s|--skip-build)
    SKIP_BUILD=yes
    shift # past argument
    ;;
    -n|--NAMESPACE)
    TEST_NS=$2
    shift # past argument
    shift # past value
    ;;
    -t|--tag)
    tag="$2"
    shift # past argument
    shift # past value
    ;;
    --default)
    DEFAULT=YES
    shift # past argument
    ;;
    *)    # unknown option
    POSITIONAL+=("$1") # save it in an array for later
    shift # past argument
    ;;
esac
done

if [ $SKIP_BUILD == "no" ]; then
    echo "Building binary"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-w" -o bin/manager ./cmd/main.go

    echo "Building docker image"
    docker build -t $IMG  -f deploy/Dockerfile bin/
    echo "Push images"
    docker push $IMG
    echo "Generating yaml"

fi
sed -e 's@image: .*@image: '"${IMG}"'@' -e 's/kube-system/'"$TEST_NS"'/g' deploy/kube-cloud-controller-manager.yaml > $DEST

kubectl create ns $TEST_NS
if [ $ACCESS_KEY_ID != "" ]; then
    printf "qy_access_key_id: '%s'\nqy_secret_access_key: '%s'\nzone: 'ap2a'\n" $ACCESS_KEY_ID $SECRET_ACCESS_KEY > /tmp/config.yaml
    secret_file=/tmp/config.yaml
fi

kubectl create secret generic qcsecret --from-file=$secret_file -n $TEST_NS
kubectl apply -f $DEST
export TEST_NS

go test -v -mod=vendor ./test/pkg/e2e/