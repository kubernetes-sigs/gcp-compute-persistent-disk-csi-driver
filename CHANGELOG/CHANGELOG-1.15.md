# v1.15.2 - Changelog since v1.15.1

- [release-1.15] Map RESOURCE_OPERATION_RATE_EXCEEDED to ResourceExhausted by @k8s-infra-cherrypick-robot in #1848
- [release-1.15] Add StatusConflict http kind to userErrorCodeMap. by @k8s-infra-cherrypick-robot in #1850
- Automated cherry pick of #1826: Add ControllerModifyVolume E2E tests
- #1836: Create documentation for ControllerModifyVolume and controller default
- #1838: Enable VolumeAttributesClass feature gate for CI runs


# v1.15.1 - Changelog since v1.15.0
### Bug
- Update base image to bookworm-v1.0.4-gke.2 by @k8s-infra-cherrypick-robot in #1833

# v1.15.0 - Changelog since v1.14.3

## Changes by Kind

### Bug or Regression
- Adding missing libgpg-error.so.0 required by nvme-cli by @pwschuurman in #1760
- Format byte array error output from google_nvme_id as string by @pwschuurman in #1761

### Feature
- Add ControllerModifyVolume functionality by @travisyx in #1801

### Uncategorized
- Add verify-docker-deps.sh to verify-all.sh by @pwschuurman in #1762
- Change OPERATION_CANCELED_BY_USER to Canceled instead of Aborted by @amacaskill in #1789
- Bump the onsi group across 1 directory with 2 updates by @dependabot in #1811
- Upgrade sanity tests to v5.3.0 and CSI Spec to v1.10.0 by @travisyx in #1814
- Add manual deployment instructions for Storage Pools by @amacaskill in #1817
- Update stable rc master image to point to v1.14.2-rc1 by @amacaskill in #1806
- Bump golang from 1.22.5 to 1.23.0 by @dependabot in #1808
- Bump golang from 1.22.4 to 1.22.5 by @dependabot in #1778
- prune changelog for 1.14 by @pwschuurman in #1745
- Add back CHANGELOG, removed in #1745 by mistake by @pwschuurman in #1746
- Add support for running tests on confidential VMs that use NVMe by @pwschuurman in #1636
