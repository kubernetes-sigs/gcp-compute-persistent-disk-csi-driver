# v1.17.12 - Changelog since v1.17.11
- [Bump up golang.org version to 0.39](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2071)

# v1.17.11 - Changelog since v1.17.10
- [Remove no-op Watcher logs for GKE Data Cache in 1.17 release branch](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2069)

# v1.17.10 - Changelog since v1.17.9
- [Fix hyperdisk attach limits](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2060)
- [Fix Hyperdisk Resize That Requires Iops/Throughput Adjustment](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2061)

# v1.17.9 - Changelog since v1.17.8
- [Relax volumeContentSource restriction for ROX multi-zone dynamic volume creation](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2036)

# v1.17.8 - Changelog since v1.17.7
- [Automated cherry pick of #2037: update cache logic to calculate chunk size based on total](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2039)

# v1.17.7 - Changelog since v1.17.6
- [Add Attach Limit for Hyperdisk + Gen4 VMs](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2021)
- [Add pageToken response check to ListSnapshots gRPC API](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2020)
- [fix outdated metadata error in watcher](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2029)

# v1.17.6 - Changelog since v1.17.5
- [Use strings.Fields for whitespace splitting to fix issues with strings.Split in case of multiple consecutive spaces](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2008)
- [Fix units for cache size while calculating chunk size for LVM](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2012)
- [Map insufficient free space error during cache creating to InvalidArgument error](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/2011)

# v1.17.5 - Changelog since v1.17.4
- [Add handling for TPC to GetRegionFromZones](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1998)

# v1.17.4 - Changelog since v1.17.3
- [Update changelog and test image to release-v1.17.1](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1971)
- [Fix CVE-2025-22870](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1991)
- [Pass performance parameters when creating regional disks.](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1992)
- [Fix logic bug while checking available LSSDs for RAIDing for Data Cache](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1994)
- [Add exponential backoff retries for getting Node from API server logic](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1995)

# v1.17.3 - Changelog since v1.17.2
- [Bump the onsi group across 1 directory with 2 updates](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1924)
- [Adding additional checks for data cache watcher and reduceVolumeGroup](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1966)
- [Fix build issues for Windows image](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1953)
- [Bump golang from 1.23.0 to 1.24.0](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1943)
- [Bump the golang-x group across 1 directory with 10 updates](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1963)
- [Fix CVE-2022-1996](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1967)
- [Pass performance parameters when creating regional disks](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1989)
- [Update change log and docs for release v1.17.1 to master](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1973)

# v1.17.2 - Changelog since v1.17.1
- [Fixed CVE-2022-1996](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1977)

# v1.17.1 - Changelog since v1.17.0
- [Setup caching only for designated node pools](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1966)

# v1.17.0 - Changelog since v1.16.1
- [Data cache feature support for PDCSI driver](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1932)
- [Handle reboot scenarios for cached nodes](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1946)
- [Add data cache error code](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1948)
- [Update RAID logic and post RAID integration](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1950)
- [Fix chunksize bug for large cache size](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1954)
- [Distinguish between user errors](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1960)