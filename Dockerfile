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

FROM --platform=$BUILDPLATFORM golang:1.17.2 as builder

ARG STAGINGVERSION
ARG TARGETPLATFORM

WORKDIR /go/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
ADD . .
RUN GOARCH=$(echo $TARGETPLATFORM | cut -f2 -d '/') GCE_PD_CSI_STAGING_VERSION=$STAGINGVERSION make gce-pd-driver

# Start from Kubernetes Debian base.
FROM k8s.gcr.io/build-image/debian-base:buster-v1.9.0 as debian
# Install necessary dependencies
# google_nvme_id script depends on the following packages: nvme-cli, xxd, bash
RUN clean-install util-linux e2fsprogs mount ca-certificates udev xfsprogs nvme-cli xxd bash
# Since we're leveraging apt to pull in dependencies, we use `gcr.io/distroless/base` because it includes glibc.
FROM gcr.io/distroless/base-debian11
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

# Copy x86 shared libraries into distroless base.
COPY --from=debian /lib/x86_64-linux-gnu/libblkid.so.1 /lib/x86_64-linux-gnu/libblkid.so.1
COPY --from=debian /lib/x86_64-linux-gnu/libcom_err.so.2 /lib/x86_64-linux-gnu/libcom_err.so.2
COPY --from=debian /lib/x86_64-linux-gnu/libext2fs.so.2 /lib/x86_64-linux-gnu/libext2fs.so.2
COPY --from=debian /lib/x86_64-linux-gnu/libe2p.so.2 /lib/x86_64-linux-gnu/libe2p.so.2
COPY --from=debian /lib/x86_64-linux-gnu/libmount.so.1 /lib/x86_64-linux-gnu/libmount.so.1
COPY --from=debian /lib/x86_64-linux-gnu/libpcre.so.3 /lib/x86_64-linux-gnu/libpcre.so.3
COPY --from=debian /lib/x86_64-linux-gnu/libreadline.so.5 /lib/x86_64-linux-gnu/libreadline.so.5
COPY --from=debian /lib/x86_64-linux-gnu/libselinux.so.1 /lib/x86_64-linux-gnu/libselinux.so.1
COPY --from=debian /lib/x86_64-linux-gnu/libtinfo.so.6 /lib/x86_64-linux-gnu/libtinfo.so.6
COPY --from=debian /lib/x86_64-linux-gnu/libuuid.so.1 /lib/x86_64-linux-gnu/libuuid.so.1

# Copy NVME support required script and rules into distroless base.
COPY deploy/kubernetes/udev/google_nvme_id /lib/udev_containerized/google_nvme_id

ENTRYPOINT ["/gce-pd-csi-driver"]