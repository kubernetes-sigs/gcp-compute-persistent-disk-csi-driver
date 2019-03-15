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

STAGINGIMAGE=${GCE_PD_CSI_STAGING_IMAGE}
STAGINGVERSION=${GCE_PD_CSI_STAGING_VERSION}

PRODIMAGE=gcr.io/gke-release/gcp-compute-persistent-disk-csi-driver
PRODVERSION=v0.4.0

all: gce-pd-driver

gce-pd-driver:
	mkdir -p bin
ifndef GCE_PD_CSI_STAGING_VERSION
	$(error "Must set environment variable GCE_PD_CSI_STAGING_VERSION to staging version")
endif
	go build -ldflags "-X main.vendorVersion=${STAGINGVERSION}" -o bin/gce-pd-csi-driver ./cmd/

build-container:
ifndef GCE_PD_CSI_STAGING_IMAGE
	$(error "Must set environment variable GCE_PD_CSI_STAGING_IMAGE to staging image repository")
endif
ifndef GCE_PD_CSI_STAGING_VERSION
	$(error "Must set environment variable GCE_PD_CSI_STAGING_VERSION to staging version")
endif
	docker build --build-arg TAG=$(STAGINGVERSION) -t $(STAGINGIMAGE):$(STAGINGVERSION) .

push-container: build-container
	gcloud docker -- push $(STAGINGIMAGE):$(STAGINGVERSION)

prod-build-container:
	docker build --build-arg TAG=$(PRODVERSION) -t $(PRODIMAGE):$(PRODVERSION) .

prod-push-container: prod-build-container
	gcloud docker -- push $(PRODIMAGE):$(PRODVERSION)

test-sanity: gce-pd-driver
	go test -timeout 30s sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/test -run ^TestSanity$

test-k8s-integration:
	go build -o bin/k8s-integration-test ./test/k8s-integration
