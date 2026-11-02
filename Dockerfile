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

FROM --platform=$BUILDPLATFORM golang:1.26.10 AS builder

ARG STAGINGVERSION
ARG TARGETPLATFORM

WORKDIR /go/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver
ADD . .
RUN GOARCH=$(echo $TARGETPLATFORM | cut -f2 -d '/') GCE_PD_CSI_STAGING_VERSION=$STAGINGVERSION make gce-pd-driver && \
    mkdir -p /driver-root/usr/lib/udev_containerized && \
    cp bin/gce-pd-csi-driver initialize-driver.sh /driver-root/ && \
    cp deploy/kubernetes/udev/google_nvme_id /driver-root/usr/lib/udev_containerized/google_nvme_id

# Start from Kubernetes Debian base.
# Updating Debian versions often breaks the driver, and should be done carefully and well in advance of EOL so that
# changes can be soaked. When the debian version is updated here, the version used in the kubernetes-csi sidecars
# should be updated as well.
FROM gke.gcr.io/debian-base:trixie-v1.0.0-gke.2 AS debian

# Install necessary dependencies
# google_nvme_id script depends on the following packages: nvme-cli, xxd, bash
RUN clean-install util-linux e2fsprogs mount ca-certificates udev xfsprogs nvme-cli xxd bash kmod lvm2 mdadm btrfs-progs

# Assemble a staging rootfs directory in a single pass so that:
# - Symlinks (such as /sbin/lv*, /sbin/pv*, /sbin/vg* -> lvm) are preserved instead of dereferenced.
# - Hardlinks (such as e2fsck/fsck.ext2/3/4, mke2fs/mkfs.ext2/3/4, dmsetup/dmstats) are preserved.
# - Merged-/usr paths (/bin, /sbin, /lib) resolve to canonical /usr directories matching distroless.
RUN set -eux; \
    LIB_DIR_PREFIX="$(uname -m)"; \
    mkdir -p \
      /staging/etc \
      /staging/lvm-tmp \
      /staging/usr/bin \
      /staging/usr/sbin \
      "/staging/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/systemd" \
      /staging/usr/lib/udev_containerized; \
    ln -s usr/bin /staging/bin; \
    ln -s usr/sbin /staging/sbin; \
    ln -s usr/lib /staging/lib; \
    cd / && cp -a --parents -t /staging \
      etc/mke2fs.conf \
      bin/mount \
      bin/umount \
      bin/btrfs \
      sbin/blkid \
      sbin/blockdev \
      sbin/dumpe2fs \
      sbin/e* \
      sbin/fsck* \
      lib/systemd/system/blk-availability.service \
      lib/systemd/system/lvm2-lvmpolld.service \
      lib/systemd/system/lvm2-lvmpolld.socket \
      lib/systemd/system/lvm2-monitor.service \
      sbin/fsadm \
      usr/lib/tmpfiles.d \
      sbin/lv* \
      sbin/pv* \
      sbin/vg* \
      bin/lsblk \
      sbin/modprobe \
      lib/udev \
      sbin/blkdeactivate \
      sbin/dmsetup \
      sbin/dmstats \
      bin/ls \
      sbin/mdadm \
      sbin/mke2fs \
      sbin/mkfs* \
      sbin/resize2fs \
      usr/libexec/xfsprogs/xfs* \
      usr/sbin/xfs* \
      usr/sbin/nvme \
      usr/bin/xxd \
      bin/bash \
      bin/date \
      bin/grep \
      bin/sed \
      bin/ln \
      bin/cp \
      bin/udevadm; \
    cp -a /etc/lvm /staging/lvm-tmp/lvm; \
    cp -a /lib/udev/scsi_id /staging/usr/lib/udev_containerized/scsi_id; \
    for link in $(find /staging/usr/bin /staging/usr/sbin /staging/usr/libexec -type l); do \
      target="$(readlink "$link")"; \
      if [ "${target#/}" != "$target" ]; then \
        check_path="/staging$target"; \
      else \
        check_path="$(dirname "$link")/$target"; \
      fi; \
      if [ ! -e "$check_path" ]; then \
        cp --remove-destination -L "/${link#/staging/}" "$link"; \
      fi; \
    done; \
    cp -L -t "/staging/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libacl.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libaio.so.1t64" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libattr.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libaudit.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libblkid.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libbsd.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libc.so.6" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libcap-ng.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libcap.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libcom_err.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libcrypt.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libcrypto.so.3" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libdbus-1.so.3" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libdevmapper-event.so.1.02.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libdevmapper.so.1.02.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libdl.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libe2p.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libedit.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libext2fs.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libgcc_s.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libicudata.so.76" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libicui18n.so.76" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libicuuc.so.76" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libinih.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libjson-c.so.5" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libkeyutils.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libkmod.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/liblz4.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/liblzma.so.5" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/liblzo2.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libm.so.6" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libmd.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libmount.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libnvme-mi.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libnvme.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libpam.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libpcre2-8.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libpthread.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libreadline.so.8" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libseccomp.so.2" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libselinux.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libsmartcols.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libstdc++.so.6" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libsystemd.so.0" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libtinfo.so.6" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libudev.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/liburcu.so.8" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libuuid.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libz.so.1" \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/libzstd.so.1"; \
    cp -L \
      "/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/systemd/libsystemd-shared-257.so" \
      "/staging/usr/lib/${LIB_DIR_PREFIX}-linux-gnu/systemd/"; \
    rm /staging/bin /staging/sbin /staging/lib

# Since we're leveraging apt to pull in dependencies, we use `gcr.io/distroless/base` because it includes glibc.
FROM gcr.io/distroless/base-debian13 AS distroless-base

# The distroless amd64 image has a target triplet of x86_64
FROM distroless-base AS distroless-amd64
ENV LIB_DIR_PREFIX=x86_64

# The distroless arm64 image has a target triplet of aarch64
FROM distroless-base AS distroless-arm64
ENV LIB_DIR_PREFIX=aarch64

FROM distroless-$TARGETARCH

# Copy staged Debian packages and shared libraries into distroless base.
COPY --from=debian /staging/ /

# Copy driver binary, entrypoint script, and NVMe udev helper into distroless base.
COPY --from=builder /driver-root/ /

ENTRYPOINT ["/initialize-driver.sh"]
