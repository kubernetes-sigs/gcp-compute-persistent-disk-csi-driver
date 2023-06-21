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

ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM golang:1.19.10 as builder

ARG STAGINGVERSION
ARG TARGETPLATFORM

WORKDIR /go/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
ADD . .
RUN GOARCH=$(echo $TARGETPLATFORM | cut -f2 -d '/') GCE_PD_CSI_STAGING_VERSION=$STAGINGVERSION make gce-pd-driver

# Start from Kubernetes Debian base.
FROM gke.gcr.io/debian-base:bullseye-v1.4.3-gke.5 as debian
# Install necessary dependencies
# google_nvme_id script depends on the following packages: nvme-cli, xxd, bash
RUN clean-install util-linux e2fsprogs mount ca-certificates udev xfsprogs nvme-cli xxd bash

# Since we're leveraging apt to pull in dependencies, we use `gcr.io/distroless/base` because it includes glibc.
FROM gcr.io/distroless/base-debian11 as distroless-base

# The distroless amd64 image has a target triplet of x86_64
FROM distroless-base AS distroless-amd64
ENV LIB_DIR_PREFIX x86_64

# The distroless arm64 image has a target triplet of aarch64
FROM distroless-base AS distroless-arm64
ENV LIB_DIR_PREFIX aarch64

FROM distroless-$TARGETARCH as output-image

# Copy necessary dependencies into distroless base.
COPY --from=builder /go/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/bin/gce-pd-csi-driver /gce-pd-csi-driver
COPY --from=debian /etc/mke2fs.conf /etc/mke2fs.conf
COPY --from=debian /lib/udev/scsi_id /lib/udev_containerized/scsi_id
COPY --from=debian /bin/mount /bin/mount
COPY --from=debian /bin/umount /bin/umount
COPY --from=debian /sbin/blkid /sbin/blkid
COPY --from=debian /sbin/blockdev /sbin/blockdev
COPY --from=debian /sbin/dumpe2fs /sbin/dumpe2fs
COPY --from=debian /sbin/e* /sbin/
COPY --from=debian /sbin/e2fsck /sbin/e2fsck
COPY --from=debian /sbin/fsck /sbin/fsck
COPY --from=debian /sbin/fsck* /sbin/
COPY --from=debian /sbin/fsck.xfs /sbin/fsck.xfs
COPY --from=debian /sbin/mke2fs /sbin/mke2fs
COPY --from=debian /sbin/mkfs* /sbin/
COPY --from=debian /sbin/resize2fs /sbin/resize2fs
COPY --from=debian /sbin/xfs_repair /sbin/xfs_repair
COPY --from=debian /usr/include/xfs /usr/include/xfs
COPY --from=debian /usr/lib/xfsprogs/xfs* /usr/lib/xfsprogs/
COPY --from=debian /usr/sbin/xfs* /usr/sbin/
# Add dependencies for /lib/udev_containerized/google_nvme_id script
COPY --from=debian /usr/sbin/nvme /usr/sbin/nvme
COPY --from=debian /usr/bin/xxd /usr/bin/xxd
COPY --from=debian /bin/bash /bin/bash
COPY --from=debian /bin/date /bin/date
COPY --from=debian /bin/grep /bin/grep
COPY --from=debian /bin/sed /bin/sed
COPY --from=debian /bin/ln /bin/ln
COPY --from=debian /bin/udevadm /bin/udevadm

# Copy shared libraries into distroless base.
COPY --from=debian /lib/${LIB_DIR_PREFIX}-linux-gnu/libpcre.so.3 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libselinux.so.1 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libtinfo.so.6 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libe2p.so.2 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libcom_err.so.2 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libdevmapper.so.1.02.1 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libext2fs.so.2 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libgcc_s.so.1 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/liblzma.so.5 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libreadline.so.8 \
                   /lib/${LIB_DIR_PREFIX}-linux-gnu/libz.so.1 /lib/${LIB_DIR_PREFIX}-linux-gnu/

COPY --from=debian /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libblkid.so.1 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libbsd.so.0 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libinih.so.1 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libmount.so.1 \         
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libudev.so.1 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libuuid.so.1 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libacl.so.1 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libattr.so.1 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libedit.so.2 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libicudata.so.67 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libicui18n.so.67 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libicuuc.so.67 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libkmod.so.2 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libmd.so.0 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libpcre2-8.so.0 \
                   /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libstdc++.so.6 /usr/lib/${LIB_DIR_PREFIX}-linux-gnu/

# Copy NVME support required script and rules into distroless base.
COPY deploy/kubernetes/udev/google_nvme_id /lib/udev_containerized/google_nvme_id

# Build stage used for validation of the output-image
# See validate-container-linux-* targets in Makefile
FROM output-image as validation-image

COPY --from=debian /usr/bin/ldd /usr/bin/find /usr/bin/xargs /usr/bin/
COPY --from=builder /go/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/hack/print-missing-deps.sh /print-missing-deps.sh
SHELL ["/bin/bash", "-c"]
RUN /print-missing-deps.sh

# Final build stage, create the real Docker image with ENTRYPOINT
FROM output-image

ENTRYPOINT ["/gce-pd-csi-driver"]
