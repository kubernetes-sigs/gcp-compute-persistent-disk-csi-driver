# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Args:
# GCE_PD_CSI_STAGING_IMAGE: Staging image repository
REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)
ifdef GCE_PD_CSI_STAGING_VERSION
	STAGINGVERSION=${GCE_PD_CSI_STAGING_VERSION}
else
	STAGINGVERSION=${REV}
endif
STAGINGIMAGE=${GCE_PD_CSI_STAGING_IMAGE}
DRIVERBINARY=gce-pd-csi-driver
DRIVERWINDOWSBINARY=${DRIVERBINARY}.exe

DOCKER=DOCKER_CLI_EXPERIMENTAL=enabled docker

all: gce-pd-driver gce-pd-driver-windows
gce-pd-driver:
	mkdir -p bin
	go build -mod=vendor -ldflags "-X main.version=${STAGINGVERSION}" -o bin/${DRIVERBINARY} ./cmd/gce-pd-csi-driver/

gce-pd-driver-windows:
	mkdir -p bin
	GOOS=windows go build -mod=vendor -ldflags -X=main.version=$(STAGINGVERSION) -o bin/${DRIVERWINDOWSBINARY} ./cmd/gce-pd-csi-driver/

build-container: require-GCE_PD_CSI_STAGING_IMAGE
	$(DOCKER) build --build-arg TAG=$(STAGINGVERSION) -t $(STAGINGIMAGE):$(STAGINGVERSION) .

build-and-push-windows-container-ltsc2019: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --file=Dockerfile.Windows --platform=windows \
		-t $(STAGINGIMAGE)-ltsc2019:$(STAGINGVERSION) --build-arg BASE_IMAGE=servercore \
		--build-arg BASE_IMAGE_TAG=ltsc2019 \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) --push .

build-and-push-windows-container-1909: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --file=Dockerfile.Windows --platform=windows \
		-t $(STAGINGIMAGE)-1909:$(STAGINGVERSION) --build-arg BASE_IMAGE=servercore \
		--build-arg BASE_IMAGE_TAG=1909 \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) --push .

build-and-push-multi-arch: build-and-push-container-linux build-and-push-windows-container-ltsc2019 build-and-push-windows-container-1909
	$(DOCKER) manifest create --amend $(STAGINGIMAGE):$(STAGINGVERSION) ${STAGINGIMAGE}-linux:${STAGINGVERSION} ${STAGINGIMAGE}-1909:${STAGINGVERSION} ${STAGINGIMAGE}-ltsc2019:${STAGINGVERSION}
	STAGINGIMAGE=${STAGINGIMAGE} STAGINGVERSION=${STAGINGVERSION} ./manifest_osversion.sh
	$(DOCKER) manifest push -p $(STAGINGIMAGE):$(STAGINGVERSION)

push-container: build-container
	gcloud docker -- push $(STAGINGIMAGE):$(STAGINGVERSION)

build-and-push-container-linux: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --platform=linux \
		-t $(STAGINGIMAGE)-linux:$(STAGINGVERSION) \
		--build-arg TAG=$(STAGINGVERSION) --push .


test-sanity: gce-pd-driver
	go test -mod=vendor --v -timeout 30s sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/sanity -run ^TestSanity$

test-k8s-integration:
	go build -mod=vendor -o bin/k8s-integration-test ./test/k8s-integration

require-GCE_PD_CSI_STAGING_IMAGE:
ifndef GCE_PD_CSI_STAGING_IMAGE
	$(error "Must set environment variable GCE_PD_CSI_STAGING_IMAGE to staging image repository")
endif

init-buildx:
	# Ensure we use a builder that can leverage it (the default on linux will not)
	-$(DOCKER) buildx rm windows-builder
	$(DOCKER) buildx create --use --name=windows-builder
	# Register gcloud as a Docker credential helper.
	# Required for "docker buildx build --push".
	gcloud auth configure-docker --quiet
