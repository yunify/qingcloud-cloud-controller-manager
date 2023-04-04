# tab space is 4
# GitHub viewer defaults to 8, change with ?ts=4 in URL

GIT_REPOSITORY= github.com/yunify/qingcloud-cloud-controller-manager
IMG?= qingcloud/cloud-controller-manager:v1.4.18
#Debug level: 0, 1, 2 (1 true, 2 use bash)
DEBUG?= 0
DOCKERFILE?= deploy/Dockerfile
TARGET?= default
DEPLOY?= deploy/kube-cloud-controller-manager.yaml


ifneq ($(DEBUG), 0)
	DOCKERFILE = deploy/Dockerfile.debug
	TARGET = dev
endif

# default just build binary
default: build

# perform go build on project
build: bin/qingcloud-cloud-controller-manager

bin/qingcloud-cloud-controller-manager:
ifeq ($(DEBUG), 0)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-w" -o bin/manager ./cmd/main.go
else
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags "all=-N -l" -o bin/manager ./cmd/main.go
endif

publish: test build
	docker build -t ${IMG}  -f ${DOCKERFILE} bin/
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' config/${TARGET}/manager_image_patch.yaml
	docker push ${IMG}
	kustomize build config/${TARGET} > ${DEPLOY}

clean:
	rm -rf bin/

test: fmt vet
	go test -v -cover  ./pkg/...
fmt:
	go fmt ./pkg/... ./cmd/...

vet:
	go vet ./pkg/... ./cmd/...

.PHONY : clean

push:
	docker buildx build -t ${IMG} --platform=linux/amd64,linux/arm64 -f deploy/DockerfileWithBuilder . --push