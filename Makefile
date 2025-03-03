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
GCE_PD_CSI_STAGING_VERSION ?= ${REV}
STAGINGVERSION=${GCE_PD_CSI_STAGING_VERSION}
STAGINGIMAGE=${GCE_PD_CSI_STAGING_IMAGE}
DRIVERBINARY=gce-pd-csi-driver
DRIVERWINDOWSBINARY=${DRIVERBINARY}.exe

DOCKER=DOCKER_CLI_EXPERIMENTAL=enabled docker

BASE_IMAGE_LTSC2019=mcr.microsoft.com/windows/servercore:ltsc2019

# Both arrays MUST be index aligned.
WINDOWS_IMAGE_TAGS=ltsc2019
WINDOWS_BASE_IMAGES=$(BASE_IMAGE_LTSC2019)

GCFLAGS=""
ifdef GCE_PD_CSI_DEBUG
	GCFLAGS="all=-N -l"
endif

all: gce-pd-driver gce-pd-driver-windows
gce-pd-driver: require-GCE_PD_CSI_STAGING_VERSION
	mkdir -p bin
	CGO_ENABLED=0 go build -mod=vendor -gcflags=$(GCFLAGS) -ldflags "-extldflags=static -X main.version=$(STAGINGVERSION)" -o bin/${DRIVERBINARY} ./cmd/gce-pd-csi-driver/

gce-pd-driver-windows: require-GCE_PD_CSI_STAGING_VERSION
ifeq (${GOARCH}, amd64)
	mkdir -p bin
	GOOS=windows go build -mod=vendor -ldflags -X=main.version=$(STAGINGVERSION) -o bin/${DRIVERWINDOWSBINARY} ./cmd/gce-pd-csi-driver/
else
	$(warning gcp-pd-driver-windows only supports amd64.)
endif

build-container: require-GCE_PD_CSI_STAGING_IMAGE require-GCE_PD_CSI_STAGING_VERSION init-buildx
	$(DOCKER) buildx build --platform=linux --progress=plain \
		-t $(STAGINGIMAGE):$(STAGINGVERSION) \
		--build-arg BUILDPLATFORM=linux \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) \
	  --push .

build-and-push-windows-container-ltsc2019: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --file=Dockerfile.Windows --platform=windows \
		-t $(STAGINGIMAGE):$(STAGINGVERSION)_ltsc2019 \
		--build-arg BASE_IMAGE=$(BASE_IMAGE_LTSC2019) \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) --push .

build-and-push-multi-arch: build-and-push-container-linux-amd64 build-and-push-container-linux-arm64 build-and-push-windows-container-ltsc2019
	$(DOCKER) manifest create $(STAGINGIMAGE):$(STAGINGVERSION) $(STAGINGIMAGE):$(STAGINGVERSION)_linux_amd64 $(STAGINGIMAGE):$(STAGINGVERSION)_linux_arm64 $(STAGINGIMAGE):$(STAGINGVERSION)_ltsc2019
	STAGINGIMAGE="$(STAGINGIMAGE)" STAGINGVERSION="$(STAGINGVERSION)" WINDOWS_IMAGE_TAGS="$(WINDOWS_IMAGE_TAGS)" WINDOWS_BASE_IMAGES="$(WINDOWS_BASE_IMAGES)" ./manifest_osversion.sh
	$(DOCKER) manifest push -p $(STAGINGIMAGE):$(STAGINGVERSION)

build-and-push-multi-arch-debug: build-and-push-container-linux-debug build-and-push-windows-container-ltsc2019
	$(DOCKER) manifest create $(STAGINGIMAGE):$(STAGINGVERSION) $(STAGINGIMAGE):$(STAGINGVERSION)_linux $(STAGINGIMAGE):$(STAGINGVERSION)_ltsc2019
	STAGINGIMAGE="$(STAGINGIMAGE)" STAGINGVERSION="$(STAGINGVERSION)" WINDOWS_IMAGE_TAGS="ltsc2019" WINDOWS_BASE_IMAGES="$(BASE_IMAGE_LTSC2019)" ./manifest_osversion.sh
	$(DOCKER) manifest push -p $(STAGINGIMAGE):$(STAGINGVERSION)

push-container: build-container

# Used by hack/verify-docker-deps.sh, not used for building artifacts
validate-container-linux-amd64: init-buildx
	$(DOCKER) buildx build --platform=linux/amd64 \
		-t validation_linux_amd64 \
		--target validation-image \
		--build-arg BUILDPLATFORM=linux \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) .

# Used by hack/verify-docker-deps.sh, not used for building artifacts
validate-container-linux-arm64: init-buildx
	$(DOCKER) buildx build --platform=linux/arm64 \
		-t validation_linux_arm64 \
		--target validation-image \
		--build-arg BUILDPLATFORM=linux \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) .

build-and-push-container-linux-amd64: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --platform=linux/amd64 \
		-t $(STAGINGIMAGE):$(STAGINGVERSION)_linux_amd64 \
		--build-arg BUILDPLATFORM=linux \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) --push .

build-and-push-container-linux-arm64: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --file=Dockerfile --platform=linux/arm64 \
		-t $(STAGINGIMAGE):$(STAGINGVERSION)_linux_arm64 \
		--build-arg BUILDPLATFORM=linux \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) --push .

build-and-push-container-linux-debug: require-GCE_PD_CSI_STAGING_IMAGE init-buildx
	$(DOCKER) buildx build --file=Dockerfile.debug --platform=linux \
		-t $(STAGINGIMAGE):$(STAGINGVERSION)_linux \
		--build-arg BUILDPLATFORM=linux \
		--build-arg STAGINGVERSION=$(STAGINGVERSION) --push .

test-sanity: gce-pd-driver
	go test -mod=vendor --v -timeout 30s sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/test/sanity -run ^TestSanity$

test-k8s-integration:
	go build -mod=vendor -o bin/k8s-integration-test ./test/k8s-integration

require-GCE_PD_CSI_STAGING_IMAGE:
ifndef GCE_PD_CSI_STAGING_IMAGE
	$(error "Must set environment variable GCE_PD_CSI_STAGING_IMAGE to staging image repository")
endif

require-GCE_PD_CSI_STAGING_VERSION:
ifndef GCE_PD_CSI_STAGING_VERSION
	$(error "Must set environment variable GCE_PD_CSI_STAGING_VERSION to build a runnable driver")
endif

init-buildx:
	$(DOCKER) run --rm --privileged multiarch/qemu-user-static --reset --credential yes --persistent yes
	# Ensure we use a builder that can leverage it (the default on linux will not)
	-$(DOCKER) buildx rm multiarch-multiplatform-builder
	$(DOCKER) buildx create --use --name=multiarch-multiplatform-builder --driver-opt network=host --driver-opt image=moby/buildkit:v0.20.0
	# Register gcloud as a Docker credential helper.
	# Required for "docker buildx build --push".
	gcloud auth configure-docker --quiet
