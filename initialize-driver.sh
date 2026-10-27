#!/bin/bash

/bin/cp -r /lvm-tmp/lvm /etc/
/bin/sed -i -e "s/.*allow_mixed_block_sizes = 0.*/	allow_mixed_block_sizes = 1/" /etc/lvm/lvm.conf
/bin/sed -i -e "s/.*udev_sync = 1.*/	udev_sync = 0/" /etc/lvm/lvm.conf
/bin/sed -i -e "s/.*udev_rules = 1.*/	udev_rules = 0/" /etc/lvm/lvm.conf
/bin/sed -i -e "s/.*event_activation = 1.*/	event_activation = 0/" /etc/lvm/lvm.conf
/bin/sed -i -e "s/.*locking_dir = .*/	 locking_dir = \"\/tmp\"/" /etc/lvm/lvm.conf
/bin/sed -i -e '/^[[:space:]]*backup[[:space:]]*{/,/^[[:space:]]*}/s/^[[:space:]]*#\{0,1\}[[:space:]]*backup[[:space:]]*=[[:space:]]*[01][[:space:]]*\(#.*\)\{0,1\}$/	backup = 0/' /etc/lvm/lvm.conf
/bin/sed -i -e '/^[[:space:]]*backup[[:space:]]*{/,/^[[:space:]]*}/s/^[[:space:]]*#\{0,1\}[[:space:]]*archive[[:space:]]*=[[:space:]]*[01][[:space:]]*\(#.*\)\{0,1\}$/	archive = 0/' /etc/lvm/lvm.conf

/gce-pd-csi-driver "$@"