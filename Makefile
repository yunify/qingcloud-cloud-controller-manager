# tab space is 4
# GitHub viewer defaults to 8, change with ?ts=4 in URL

# Vars describing project
NAME= qingcloud-cloud-controller-manager
GIT_REPOSITORY= github.com/yunify/qingcloud-cloud-controller-manager
IMG?= kubespheredev/cloud-controller-manager:v1.4.0

# Generate vars to be included from external script
# Allows using bash to generate complex vars, such as project versions
GENERATE_VERSION_INFO_SCRIPT	= ./generate_version.sh
GENERATE_VERSION_INFO_OUTPUT	= version_info

# Define newline needed for subsitution to allow evaluating multiline script output
define newline


endef

# Call the version_info script with keyvalue option and evaluate the output
# Will import the keyvalue pairs and make available as Makefile variables
# Use dummy variable to only have execute once
$(eval $(subst #,$(newline),$(shell $(GENERATE_VERSION_INFO_SCRIPT) keyvalue | tr '\n' '#')))
# Call the verson_info script with json option and store result into output file and variable
# Will only execute once due to ':='
#GENERATE_VERSION_INFO			:= $(shell $(GENERATE_VERSION_INFO_SCRIPT) json | tee $(GENERATE_VERSION_INFO_OUTPUT))

# Set defaults for needed vars in case version_info script did not set
# Revision set to number of commits ahead
VERSION							?= 0.0
COMMITS							?= 0
REVISION						?= $(COMMITS)
BUILD_LABEL						?= unknown_build
BUILD_DATE						?= $(shell date -u +%Y%m%d.%H%M%S)
GIT_SHA1						?= unknown_sha1

IMAGE_LABLE         ?= $(BUILD_LABEL)

# default just build binary
default							: go-build

# target for debugging / printing variables
print-%							:
								@echo '$*=$($*)'

# perform go build on project
go-build						: bin/qingcloud-cloud-controller-manager

bin/qingcloud-cloud-controller-manager                     : 
								CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-w" -o bin/manager ./cmd/main.go

bin/.docker-images-build-timestamp                         : bin/qingcloud-cloud-controller-manager Makefile Dockerfile
								docker build -q -t $(DOCKER_IMAGE_NAME):$(IMAGE_LABLE) -t dockerhub.qingcloud.com/$(DOCKER_IMAGE_NAME):$(IMAGE_LABLE) . > bin/.docker-images-build-timestamp

publish                         : test go-build
								docker build -t ${IMG}  -f deploy/Dockerfile bin/
								docker push ${IMG}
clean                           :
								rm -rf bin/ && if -f bin/.docker-images-build-timestamp then docker rmi `cat bin/.docker-images-build-timestamp`
test                            : fmt vet
								go test -v -cover -mod=vendor ./pkg/...
fmt								:
								go fmt ./pkg/... ./cmd/... ./test/pkg/...
vet:
								go vet ./pkg/... ./cmd/... ./test/pkg/...

debug:
								./hack/debug.sh
								
.PHONY							: default all go-build clean install-docker test

e2e:
								./hack/e2e.sh