#!/bin/bash
# This should be run as root
#
# This script cleans up LVM volumes on a test instance. This should not be
# used for general LVM cleanup --- it assumes data is not important, and will
# forcibly destry mounts.

set -exu

for vg_name in $(vgs -o vg_name --noheadings | grep csi-vg); do
  
  # Find and kill processes holding onto any LV in this VG LVM
  # replaces single hyphens with double hyphens in /dev/mapper/ e.g.,
  # csi-vg-wqrqwzmh becomes csi--vg--wqrqwzmh
  escaped_vg=$(echo "$vg_name" | sed 's/-/--/g')
  
  for lv_path in /dev/mapper/"${escaped_vg}"-*; do
    if [[ "$lv_path" =~ (_cvol|_cvol-cdata|_cvol-cmeta|_corig)$ ]]; then
      continue
    fi
    count=0
    # Avoid race conditions with short-lived processes by repeating
    # the umount attempts.
    while [ -e "$lv_path" ] && mount | grep -q "$lv_path"; do
      if (( count > 5 )); then
        echo "$lv_path" still mounted after "$count" tries && false
      fi
      let count+=1

      # Forcefully kill any process using the mount point or raw device
      # If there are no processes, fuser will fail, so we ignore failures.
      fuser -k -9 "$lv_path" || true
      
      # Try to unmount now that processes are dead.
      umount -f "$lv_path" || true
    done
  done

  # Deactivate the Volume Group (Forces them offline)
  # If it still complains, we force a close on the DM devices via dmsetup
  if ! vgchange -an --force "${vg_name}"; then
    echo "VG still open, forcing device-mapper removal..."
    for lv_name in $(lvs "${vg_name}" -o lv_name --noheadings); do
      dmsetup remove -f "/dev/mapper/${escaped_vg}-${lv_name}"
    done
    # Retry deactivation
    vgchange -an --force "${vg_name}"
  fi

  # Force-remove the Volume Group
  vgremove -y -f "${vg_name}"

  # Clean up the Physical Volumes
  pvs=$(pvs --noheadings -o pv_name -S "vg_name=${vg_name}" | grep -v '\[unknown\]' | xargs)
  if [ -n "$pvs" ]; then
    pvremove -y -f $pvs
    wipefs -a $pvs
  fi
done
