# v1.0.3 - Changelog since v1.0.2

### Other

- Fix build for various go versions, ([#832](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/832), [@mattcary](https://github.com/mattcary))

# v1.0.2 - Changelog since v1.0.1

### Bug or Regression

- Update base image to buster-v.1.9.0 ([#829](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/829), [@mattcary](https://github.com/mattcary))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.0.1 - Changelog since v1.0.0

### Other (Cleanup or Flake)

- Update GCE PD CSI Driver Docker base image to `k8s.gcr.io/build-image/debian-base-amd64:v2.1.3` (previously `gcr.io/google-containers/debian-base-amd64:v2.0.0`) to address CVEs. ([#598](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/598), [@saad-ali](https://github.com/saad-ali))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.0.0 - Changelog since v0.7.0

## Changes by Kind

### Feature

- Add support for multi-writer raw block devices. ([#415](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/415), [@sschmitt](https://github.com/sschmitt))
- Enable CSI Snapshotter (2.x) side car for PD CSI driver ([#500](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/500), [@saikat-royc](https://github.com/saikat-royc))
- Enable CSI snapshotter side car (beta) for PD CSI driver. ([#507](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/507), [@saikat-royc](https://github.com/saikat-royc))
- Increase sidecar operation timeout for stable overlay ([#577](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/577), [@saikat-royc](https://github.com/saikat-royc))
- Provisioned GCE PD Description will contain JSON with information about what PVC/PV disk was created for (similar to volumes provisioned by in-tree GCE PD plugin). This feature requires the `--extra-create-metadata` flag to be set on the CSI external-provisioner. ([#570](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/570), [@saad-ali](https://github.com/saad-ali))
- The driver deployment now has leader election enabled, and the controller deployment uses a Deployment instead of a StatefulSet. ([#521](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/521), [@verult](https://github.com/verult))
- Updating the following image versions in stable deployment specs:
  - csi-provisioner: v1.6.0-gke.0
  - csi-attacher: v2.2.0-gke.0
  - csi-node-driver-registrar: v1.3.0-gke.0
  - csi-resizer: v0.5.0-gke.0 ([#517](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/517), [@verult](https://github.com/verult))
- Add Windows support in GCE PD driver with the use of csi-proxy (https://github.com/kubernetes-csi/csi-proxy/). ([#483](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/483), [@jingxu97](https://github.com/jingxu97))
- Adds new build rules using docker buildx feature for allowing to build windows container image on either windows or linux node. ([#489](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/489), [@jingxu97](https://github.com/jingxu97))
- Install CSIDriver object as part of driver deployment. ([#575](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/575), [@Jiawei0227](https://github.com/Jiawei0227))

### Bug or Regression

- Increase provisioner and attacher op timeout to reduce retries ([#542](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/542), [@saikat-royc](https://github.com/saikat-royc))
- Fixed issue where provisioning of GCE PDs with CMEK used to sometimes fails with `disk already exists with same name` error. ([#563](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/563), [@saad-ali](https://github.com/saad-ali))
- Honor image-type in GKE cluster deploy ([#536](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/536), [@saikat-royc](https://github.com/saikat-royc))
- In GCE PersistentDisk CSI Driver CreateVolume call, wait for disk to reach a READY status, before returning success to caller. ([#527](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/527), [@saikat-royc](https://github.com/saikat-royc))
- Skip NodeExpandVolume for block volumes. ([#571](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/571), [@saad-ali](https://github.com/saad-ali))

### Failing Test

- Add autorepair options for GKE release channel ([#538](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/538), [@saikat-royc](https://github.com/saikat-royc))
- Disable the optional capability to handle volume in user errors for staging-head PD driver. ([#540](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/540), [@saikat-royc](https://github.com/saikat-royc))
- Add hook to deploy GKE with GCE PD CSI driver, and run kubernetes e2e tests against the managed driver ([#515](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/515), [@saikat-royc](https://github.com/saikat-royc))
- Enable PD CSI snapshot tests for release-staging-head and release-staging-rc ([#505](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/505), [@saikat-royc](https://github.com/saikat-royc))

### Documentation

- Update documentation and user guides for beta snapshotter ([#508](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/508), [@saikat-royc](https://github.com/saikat-royc))


## Dependencies

### Added
- bitbucket.org/bertimus9/systemstat: 0eeff89
- github.com/Azure/go-autorest/autorest/adal: [v0.5.0](https://github.com/Azure/go-autorest/autorest/adal/tree/v0.5.0)
- github.com/Azure/go-autorest/autorest/date: [v0.1.0](https://github.com/Azure/go-autorest/autorest/date/tree/v0.1.0)
- github.com/Azure/go-autorest/autorest/mocks: [v0.2.0](https://github.com/Azure/go-autorest/autorest/mocks/tree/v0.2.0)
- github.com/Azure/go-autorest/autorest/to: [v0.2.0](https://github.com/Azure/go-autorest/autorest/to/tree/v0.2.0)
- github.com/Azure/go-autorest/autorest/validation: [v0.1.0](https://github.com/Azure/go-autorest/autorest/validation/tree/v0.1.0)
- github.com/Azure/go-autorest/autorest: [v0.9.0](https://github.com/Azure/go-autorest/autorest/tree/v0.9.0)
- github.com/Azure/go-autorest/logger: [v0.1.0](https://github.com/Azure/go-autorest/logger/tree/v0.1.0)
- github.com/Azure/go-autorest/tracing: [v0.5.0](https://github.com/Azure/go-autorest/tracing/tree/v0.5.0)
- github.com/JeffAshton/win_pdh: [76bb4ee](https://github.com/JeffAshton/win_pdh/tree/76bb4ee)
- github.com/MakeNowJust/heredoc: [bb23615](https://github.com/MakeNowJust/heredoc/tree/bb23615)
- github.com/Microsoft/hcsshim: [672e52e](https://github.com/Microsoft/hcsshim/tree/672e52e)
- github.com/OpenPeeDeeP/depguard: [v1.0.1](https://github.com/OpenPeeDeeP/depguard/tree/v1.0.1)
- github.com/Rican7/retry: [v0.1.0](https://github.com/Rican7/retry/tree/v0.1.0)
- github.com/StackExchange/wmi: [5d04971](https://github.com/StackExchange/wmi/tree/5d04971)
- github.com/agnivade/levenshtein: [v1.0.1](https://github.com/agnivade/levenshtein/tree/v1.0.1)
- github.com/ajstarks/svgo: [644b8db](https://github.com/ajstarks/svgo/tree/644b8db)
- github.com/andreyvit/diff: [c7f18ee](https://github.com/andreyvit/diff/tree/c7f18ee)
- github.com/anmitsu/go-shlex: [648efa6](https://github.com/anmitsu/go-shlex/tree/648efa6)
- github.com/armon/circbuf: [bbbad09](https://github.com/armon/circbuf/tree/bbbad09)
- github.com/auth0/go-jwt-middleware: [5493cab](https://github.com/auth0/go-jwt-middleware/tree/5493cab)
- github.com/bazelbuild/bazel-gazelle: [70208cb](https://github.com/bazelbuild/bazel-gazelle/tree/70208cb)
- github.com/bazelbuild/rules_go: [6dae44d](https://github.com/bazelbuild/rules_go/tree/6dae44d)
- github.com/bifurcation/mint: [93c51c6](https://github.com/bifurcation/mint/tree/93c51c6)
- github.com/boltdb/bolt: [v1.3.1](https://github.com/boltdb/bolt/tree/v1.3.1)
- github.com/bradfitz/go-smtpd: [deb6d62](https://github.com/bradfitz/go-smtpd/tree/deb6d62)
- github.com/caddyserver/caddy: [v1.0.3](https://github.com/caddyserver/caddy/tree/v1.0.3)
- github.com/cenkalti/backoff: [v2.1.1+incompatible](https://github.com/cenkalti/backoff/tree/v2.1.1)
- github.com/cespare/prettybench: [03b8cfe](https://github.com/cespare/prettybench/tree/03b8cfe)
- github.com/chai2010/gettext-go: [c6fed77](https://github.com/chai2010/gettext-go/tree/c6fed77)
- github.com/checkpoint-restore/go-criu: [17b0214](https://github.com/checkpoint-restore/go-criu/tree/17b0214)
- github.com/cheekybits/genny: [9127e81](https://github.com/cheekybits/genny/tree/9127e81)
- github.com/cilium/ebpf: [95b36a5](https://github.com/cilium/ebpf/tree/95b36a5)
- github.com/clusterhq/flocker-go: [2b8b725](https://github.com/clusterhq/flocker-go/tree/2b8b725)
- github.com/cockroachdb/datadriven: [80d97fb](https://github.com/cockroachdb/datadriven/tree/80d97fb)
- github.com/codegangsta/negroni: [v1.0.0](https://github.com/codegangsta/negroni/tree/v1.0.0)
- github.com/containerd/console: [84eeaae](https://github.com/containerd/console/tree/84eeaae)
- github.com/containerd/containerd: [v1.0.2](https://github.com/containerd/containerd/tree/v1.0.2)
- github.com/containerd/typeurl: [2a93cfd](https://github.com/containerd/typeurl/tree/2a93cfd)
- github.com/containernetworking/cni: [v0.7.1](https://github.com/containernetworking/cni/tree/v0.7.1)
- github.com/coredns/corefile-migration: [v1.0.6](https://github.com/coredns/corefile-migration/tree/v1.0.6)
- github.com/creack/pty: [v1.1.7](https://github.com/creack/pty/tree/v1.1.7)
- github.com/cyphar/filepath-securejoin: [v0.2.2](https://github.com/cyphar/filepath-securejoin/tree/v0.2.2)
- github.com/daviddengcn/go-colortext: [511bcaf](https://github.com/daviddengcn/go-colortext/tree/511bcaf)
- github.com/dnaeon/go-vcr: [v1.0.1](https://github.com/dnaeon/go-vcr/tree/v1.0.1)
- github.com/docker/libnetwork: [c8a5fca](https://github.com/docker/libnetwork/tree/c8a5fca)
- github.com/euank/go-kmsg-parser: [v2.0.0+incompatible](https://github.com/euank/go-kmsg-parser/tree/v2.0.0)
- github.com/exponent-io/jsonpath: [d6023ce](https://github.com/exponent-io/jsonpath/tree/d6023ce)
- github.com/fatih/camelcase: [v1.0.0](https://github.com/fatih/camelcase/tree/v1.0.0)
- github.com/flynn/go-shlex: [3f9db97](https://github.com/flynn/go-shlex/tree/3f9db97)
- github.com/fogleman/gg: [0403632](https://github.com/fogleman/gg/tree/0403632)
- github.com/gliderlabs/ssh: [v0.1.1](https://github.com/gliderlabs/ssh/tree/v0.1.1)
- github.com/go-acme/lego: [v2.5.0+incompatible](https://github.com/go-acme/lego/tree/v2.5.0)
- github.com/go-bindata/go-bindata: [v3.1.1+incompatible](https://github.com/go-bindata/go-bindata/tree/v3.1.1)
- github.com/go-critic/go-critic: [1df3008](https://github.com/go-critic/go-critic/tree/1df3008)
- github.com/go-lintpack/lintpack: [v0.5.2](https://github.com/go-lintpack/lintpack/tree/v0.5.2)
- github.com/go-ole/go-ole: [v1.2.1](https://github.com/go-ole/go-ole/tree/v1.2.1)
- github.com/go-ozzo/ozzo-validation: [v3.5.0+incompatible](https://github.com/go-ozzo/ozzo-validation/tree/v3.5.0)
- github.com/go-toolsmith/astcast: [v1.0.0](https://github.com/go-toolsmith/astcast/tree/v1.0.0)
- github.com/go-toolsmith/astcopy: [v1.0.0](https://github.com/go-toolsmith/astcopy/tree/v1.0.0)
- github.com/go-toolsmith/astequal: [v1.0.0](https://github.com/go-toolsmith/astequal/tree/v1.0.0)
- github.com/go-toolsmith/astfmt: [v1.0.0](https://github.com/go-toolsmith/astfmt/tree/v1.0.0)
- github.com/go-toolsmith/astinfo: [9809ff7](https://github.com/go-toolsmith/astinfo/tree/9809ff7)
- github.com/go-toolsmith/astp: [v1.0.0](https://github.com/go-toolsmith/astp/tree/v1.0.0)
- github.com/go-toolsmith/pkgload: [v1.0.0](https://github.com/go-toolsmith/pkgload/tree/v1.0.0)
- github.com/go-toolsmith/strparse: [v1.0.0](https://github.com/go-toolsmith/strparse/tree/v1.0.0)
- github.com/go-toolsmith/typep: [v1.0.0](https://github.com/go-toolsmith/typep/tree/v1.0.0)
- github.com/gobwas/glob: [v0.2.3](https://github.com/gobwas/glob/tree/v0.2.3)
- github.com/godbus/dbus: [2ff6f7f](https://github.com/godbus/dbus/tree/2ff6f7f)
- github.com/golang/freetype: [e2365df](https://github.com/golang/freetype/tree/e2365df)
- github.com/golangci/check: [cfe4005](https://github.com/golangci/check/tree/cfe4005)
- github.com/golangci/dupl: [3e9179a](https://github.com/golangci/dupl/tree/3e9179a)
- github.com/golangci/errcheck: [ef45e06](https://github.com/golangci/errcheck/tree/ef45e06)
- github.com/golangci/go-misc: [927a3d8](https://github.com/golangci/go-misc/tree/927a3d8)
- github.com/golangci/go-tools: [e32c541](https://github.com/golangci/go-tools/tree/e32c541)
- github.com/golangci/goconst: [041c5f2](https://github.com/golangci/goconst/tree/041c5f2)
- github.com/golangci/gocyclo: [2becd97](https://github.com/golangci/gocyclo/tree/2becd97)
- github.com/golangci/gofmt: [0b8337e](https://github.com/golangci/gofmt/tree/0b8337e)
- github.com/golangci/golangci-lint: [v1.18.0](https://github.com/golangci/golangci-lint/tree/v1.18.0)
- github.com/golangci/gosec: [66fb7fc](https://github.com/golangci/gosec/tree/66fb7fc)
- github.com/golangci/ineffassign: [42439a7](https://github.com/golangci/ineffassign/tree/42439a7)
- github.com/golangci/lint-1: [ee948d0](https://github.com/golangci/lint-1/tree/ee948d0)
- github.com/golangci/maligned: [b1d8939](https://github.com/golangci/maligned/tree/b1d8939)
- github.com/golangci/misspell: [950f5d1](https://github.com/golangci/misspell/tree/950f5d1)
- github.com/golangci/prealloc: [215b22d](https://github.com/golangci/prealloc/tree/215b22d)
- github.com/golangci/revgrep: [d9c87f5](https://github.com/golangci/revgrep/tree/d9c87f5)
- github.com/golangci/unconvert: [28b1c44](https://github.com/golangci/unconvert/tree/28b1c44)
- github.com/golangplus/bytes: [45c989f](https://github.com/golangplus/bytes/tree/45c989f)
- github.com/golangplus/fmt: [2a5d6d7](https://github.com/golangplus/fmt/tree/2a5d6d7)
- github.com/golangplus/testing: [af21d9c](https://github.com/golangplus/testing/tree/af21d9c)
- github.com/google/cadvisor: [v0.35.0](https://github.com/google/cadvisor/tree/v0.35.0)
- github.com/google/renameio: [v0.1.0](https://github.com/google/renameio/tree/v0.1.0)
- github.com/gopherjs/gopherjs: [0766667](https://github.com/gopherjs/gopherjs/tree/0766667)
- github.com/gostaticanalysis/analysisutil: [v0.0.3](https://github.com/gostaticanalysis/analysisutil/tree/v0.0.3)
- github.com/hashicorp/go-syslog: [v1.0.0](https://github.com/hashicorp/go-syslog/tree/v1.0.0)
- github.com/heketi/heketi: [c2e2a4a](https://github.com/heketi/heketi/tree/c2e2a4a)
- github.com/heketi/tests: [f3775cb](https://github.com/heketi/tests/tree/f3775cb)
- github.com/jellevandenhooff/dkim: [f50fe3d](https://github.com/jellevandenhooff/dkim/tree/f50fe3d)
- github.com/jimstudt/http-authentication: [3eca13d](https://github.com/jimstudt/http-authentication/tree/3eca13d)
- github.com/jtolds/gls: [v4.20.0+incompatible](https://github.com/jtolds/gls/tree/v4.20.0)
- github.com/jung-kurt/gofpdf: [24315ac](https://github.com/jung-kurt/gofpdf/tree/24315ac)
- github.com/karrick/godirwalk: [v1.7.5](https://github.com/karrick/godirwalk/tree/v1.7.5)
- github.com/kubernetes-csi/csi-proxy/client: [9eff164](https://github.com/kubernetes-csi/csi-proxy/client/tree/9eff164)
- github.com/kylelemons/godebug: [d65d576](https://github.com/kylelemons/godebug/tree/d65d576)
- github.com/libopenstorage/openstorage: [v1.0.0](https://github.com/libopenstorage/openstorage/tree/v1.0.0)
- github.com/liggitt/tabwriter: [89fcab3](https://github.com/liggitt/tabwriter/tree/89fcab3)
- github.com/lithammer/dedent: [v1.1.0](https://github.com/lithammer/dedent/tree/v1.1.0)
- github.com/logrusorgru/aurora: [a7b3b31](https://github.com/logrusorgru/aurora/tree/a7b3b31)
- github.com/lpabon/godbc: [v0.1.1](https://github.com/lpabon/godbc/tree/v0.1.1)
- github.com/lucas-clemente/aes12: [cd47fb3](https://github.com/lucas-clemente/aes12/tree/cd47fb3)
- github.com/lucas-clemente/quic-clients: [v0.1.0](https://github.com/lucas-clemente/quic-clients/tree/v0.1.0)
- github.com/lucas-clemente/quic-go-certificates: [d2f8652](https://github.com/lucas-clemente/quic-go-certificates/tree/d2f8652)
- github.com/lucas-clemente/quic-go: [v0.10.2](https://github.com/lucas-clemente/quic-go/tree/v0.10.2)
- github.com/marten-seemann/qtls: [v0.2.3](https://github.com/marten-seemann/qtls/tree/v0.2.3)
- github.com/mattn/go-shellwords: [v1.0.5](https://github.com/mattn/go-shellwords/tree/v1.0.5)
- github.com/mattn/goveralls: [v0.0.2](https://github.com/mattn/goveralls/tree/v0.0.2)
- github.com/mesos/mesos-go: [v0.0.9](https://github.com/mesos/mesos-go/tree/v0.0.9)
- github.com/mholt/certmagic: [6a42ef9](https://github.com/mholt/certmagic/tree/6a42ef9)
- github.com/miekg/dns: [v1.1.4](https://github.com/miekg/dns/tree/v1.1.4)
- github.com/mindprince/gonvml: [9ebdce4](https://github.com/mindprince/gonvml/tree/9ebdce4)
- github.com/mistifyio/go-zfs: [v2.1.1+incompatible](https://github.com/mistifyio/go-zfs/tree/v2.1.1)
- github.com/mitchellh/go-ps: [4fdf99a](https://github.com/mitchellh/go-ps/tree/4fdf99a)
- github.com/mitchellh/go-wordwrap: [v1.0.0](https://github.com/mitchellh/go-wordwrap/tree/v1.0.0)
- github.com/mohae/deepcopy: [491d360](https://github.com/mohae/deepcopy/tree/491d360)
- github.com/morikuni/aec: [v1.0.0](https://github.com/morikuni/aec/tree/v1.0.0)
- github.com/mozilla/tls-observatory: [8791a20](https://github.com/mozilla/tls-observatory/tree/8791a20)
- github.com/mrunalp/fileutils: [7d4729f](https://github.com/mrunalp/fileutils/tree/7d4729f)
- github.com/mvdan/xurls: [v1.1.0](https://github.com/mvdan/xurls/tree/v1.1.0)
- github.com/naoina/go-stringutil: [v0.1.0](https://github.com/naoina/go-stringutil/tree/v0.1.0)
- github.com/naoina/toml: [v0.1.1](https://github.com/naoina/toml/tree/v0.1.1)
- github.com/nbutton23/zxcvbn-go: [eafdab6](https://github.com/nbutton23/zxcvbn-go/tree/eafdab6)
- github.com/opencontainers/runc: [v1.0.0-rc10](https://github.com/opencontainers/runc/tree/v1.0.0-rc10)
- github.com/opencontainers/runtime-spec: [v1.0.0](https://github.com/opencontainers/runtime-spec/tree/v1.0.0)
- github.com/opencontainers/selinux: [5215b18](https://github.com/opencontainers/selinux/tree/5215b18)
- github.com/pquerna/ffjson: [af8b230](https://github.com/pquerna/ffjson/tree/af8b230)
- github.com/quasilyte/go-consistent: [c6f3937](https://github.com/quasilyte/go-consistent/tree/c6f3937)
- github.com/quobyte/api: [v0.1.2](https://github.com/quobyte/api/tree/v0.1.2)
- github.com/robfig/cron: [v1.1.0](https://github.com/robfig/cron/tree/v1.1.0)
- github.com/rogpeppe/fastuuid: [6724a57](https://github.com/rogpeppe/fastuuid/tree/6724a57)
- github.com/rogpeppe/go-internal: [v1.3.0](https://github.com/rogpeppe/go-internal/tree/v1.3.0)
- github.com/rubiojr/go-vhd: [0bfd3b3](https://github.com/rubiojr/go-vhd/tree/0bfd3b3)
- github.com/ryanuber/go-glob: [256dc44](https://github.com/ryanuber/go-glob/tree/256dc44)
- github.com/seccomp/libseccomp-golang: [v0.9.1](https://github.com/seccomp/libseccomp-golang/tree/v0.9.1)
- github.com/sergi/go-diff: [v1.0.0](https://github.com/sergi/go-diff/tree/v1.0.0)
- github.com/shirou/gopsutil: [c95755e](https://github.com/shirou/gopsutil/tree/c95755e)
- github.com/shirou/w32: [bb4de01](https://github.com/shirou/w32/tree/bb4de01)
- github.com/shurcooL/go-goon: [37c2f52](https://github.com/shurcooL/go-goon/tree/37c2f52)
- github.com/smartystreets/assertions: [b2de0cb](https://github.com/smartystreets/assertions/tree/b2de0cb)
- github.com/smartystreets/goconvey: [v1.6.4](https://github.com/smartystreets/goconvey/tree/v1.6.4)
- github.com/sourcegraph/go-diff: [v0.5.1](https://github.com/sourcegraph/go-diff/tree/v0.5.1)
- github.com/storageos/go-api: [343b3ef](https://github.com/storageos/go-api/tree/343b3ef)
- github.com/syndtr/gocapability: [d983527](https://github.com/syndtr/gocapability/tree/d983527)
- github.com/tarm/serial: [98f6abe](https://github.com/tarm/serial/tree/98f6abe)
- github.com/thecodeteam/goscaleio: [v0.1.0](https://github.com/thecodeteam/goscaleio/tree/v0.1.0)
- github.com/tidwall/pretty: [v1.0.0](https://github.com/tidwall/pretty/tree/v1.0.0)
- github.com/timakin/bodyclose: [87058b9](https://github.com/timakin/bodyclose/tree/87058b9)
- github.com/ultraware/funlen: [v0.0.2](https://github.com/ultraware/funlen/tree/v0.0.2)
- github.com/urfave/negroni: [v1.0.0](https://github.com/urfave/negroni/tree/v1.0.0)
- github.com/valyala/bytebufferpool: [v1.0.0](https://github.com/valyala/bytebufferpool/tree/v1.0.0)
- github.com/valyala/fasthttp: [v1.2.0](https://github.com/valyala/fasthttp/tree/v1.2.0)
- github.com/valyala/quicktemplate: [v1.1.1](https://github.com/valyala/quicktemplate/tree/v1.1.1)
- github.com/valyala/tcplisten: [ceec8f9](https://github.com/valyala/tcplisten/tree/ceec8f9)
- github.com/vektah/gqlparser: [v1.1.2](https://github.com/vektah/gqlparser/tree/v1.1.2)
- github.com/vishvananda/netlink: [v1.0.0](https://github.com/vishvananda/netlink/tree/v1.0.0)
- github.com/vishvananda/netns: [be1fbed](https://github.com/vishvananda/netns/tree/be1fbed)
- github.com/vmware/govmomi: [v0.20.3](https://github.com/vmware/govmomi/tree/v0.20.3)
- go.mongodb.org/mongo-driver: v1.1.2
- go4.org: 417644f
- golang.org/x/build: 2835ba2
- golang.org/x/mod: c90efee
- golang.org/x/perf: 6e6d33e
- golang.org/x/tools/gopls: v0.3.3
- gonum.org/v1/plot: e2840ee
- gopkg.in/errgo.v2: v2.1.0
- gopkg.in/mcuadros/go-syslog.v2: v2.2.1
- gopkg.in/resty.v1: v1.12.0
- gotest.tools/gotestsum: v0.3.5
- grpc.go4.org: 11d0a25
- k8s.io/cli-runtime: v0.18.0
- k8s.io/cloud-provider: v0.18.0
- k8s.io/cluster-bootstrap: v0.18.0
- k8s.io/cri-api: v0.18.0
- k8s.io/csi-translation-lib: v0.18.0
- k8s.io/heapster: v1.2.0-beta.1
- k8s.io/kube-aggregator: v0.18.0
- k8s.io/kube-controller-manager: v0.18.0
- k8s.io/kube-proxy: v0.18.0
- k8s.io/kube-scheduler: v0.18.0
- k8s.io/kubectl: v0.18.0
- k8s.io/kubelet: v0.18.0
- k8s.io/legacy-cloud-providers: v0.18.0
- k8s.io/metrics: v0.18.0
- k8s.io/repo-infra: v0.0.1-alpha.1
- k8s.io/sample-apiserver: v0.18.0
- k8s.io/system-validators: v1.0.4
- mvdan.cc/interfacer: c200402
- mvdan.cc/lint: adc824a
- mvdan.cc/unparam: fbb5962
- rsc.io/pdf: v0.1.1
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.7
- sigs.k8s.io/kustomize: v2.0.3+incompatible
- sigs.k8s.io/structured-merge-diff/v3: v3.0.0
- sourcegraph.com/sqs/pbtypes: d3ebe8f

### Changed
- github.com/Azure/azure-sdk-for-go: [v21.1.0+incompatible → v35.0.0+incompatible](https://github.com/Azure/azure-sdk-for-go/compare/v21.1.0...v35.0.0)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [2e19bb3 → 27a4ced](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/2e19bb3...27a4ced)
- github.com/asaskevich/govalidator: [f9ffefc → f61b66f](https://github.com/asaskevich/govalidator/compare/f9ffefc...f61b66f)
- github.com/aws/aws-sdk-go: [v1.23.22 → v1.28.2](https://github.com/aws/aws-sdk-go/compare/v1.23.22...v1.28.2)
- github.com/coreos/etcd: [v3.3.13+incompatible → v3.3.10+incompatible](https://github.com/coreos/etcd/compare/v3.3.13...v3.3.10)
- github.com/coreos/go-oidc: [065b426 → v2.1.0+incompatible](https://github.com/coreos/go-oidc/compare/065b426...v2.1.0)
- github.com/coreos/go-semver: [v0.2.0 → v0.3.0](https://github.com/coreos/go-semver/compare/v0.2.0...v0.3.0)
- github.com/coreos/go-systemd: [39ca1b0 → 95778df](https://github.com/coreos/go-systemd/compare/39ca1b0...95778df)
- github.com/docker/distribution: [83389a1 → v2.7.1+incompatible](https://github.com/docker/distribution/compare/83389a1...v2.7.1)
- github.com/docker/go-units: [v0.3.3 → v0.4.0](https://github.com/docker/go-units/compare/v0.3.3...v0.4.0)
- github.com/elazarl/goproxy: [c4fc265 → 947c36d](https://github.com/elazarl/goproxy/compare/c4fc265...947c36d)
- github.com/envoyproxy/go-control-plane: [v0.9.0 → 5f8ba28](https://github.com/envoyproxy/go-control-plane/compare/v0.9.0...5f8ba28)
- github.com/go-openapi/analysis: [v0.17.2 → v0.19.5](https://github.com/go-openapi/analysis/compare/v0.17.2...v0.19.5)
- github.com/go-openapi/errors: [v0.17.2 → v0.19.2](https://github.com/go-openapi/errors/compare/v0.17.2...v0.19.2)
- github.com/go-openapi/loads: [v0.17.2 → v0.19.4](https://github.com/go-openapi/loads/compare/v0.17.2...v0.19.4)
- github.com/go-openapi/runtime: [v0.17.2 → v0.19.4](https://github.com/go-openapi/runtime/compare/v0.17.2...v0.19.4)
- github.com/go-openapi/strfmt: [v0.17.0 → v0.19.3](https://github.com/go-openapi/strfmt/compare/v0.17.0...v0.19.3)
- github.com/go-openapi/validate: [v0.18.0 → v0.19.5](https://github.com/go-openapi/validate/compare/v0.18.0...v0.19.5)
- github.com/gogo/protobuf: [65acae2 → v1.3.1](https://github.com/gogo/protobuf/compare/65acae2...v1.3.1)
- github.com/golang/protobuf: [v1.3.2 → v1.3.4](https://github.com/golang/protobuf/compare/v1.3.2...v1.3.4)
- github.com/google/gofuzz: [v1.0.0 → v1.1.0](https://github.com/google/gofuzz/compare/v1.0.0...v1.1.0)
- github.com/gophercloud/gophercloud: [c818fa6 → v0.1.0](https://github.com/gophercloud/gophercloud/compare/c818fa6...v0.1.0)
- github.com/gorilla/mux: [v1.6.2 → v1.7.0](https://github.com/gorilla/mux/compare/v1.6.2...v1.7.0)
- github.com/gorilla/websocket: [4201258 → v1.4.0](https://github.com/gorilla/websocket/compare/4201258...v1.4.0)
- github.com/grpc-ecosystem/go-grpc-middleware: [v1.0.0 → f849b54](https://github.com/grpc-ecosystem/go-grpc-middleware/compare/v1.0.0...f849b54)
- github.com/grpc-ecosystem/grpc-gateway: [v1.4.1 → v1.9.5](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.4.1...v1.9.5)
- github.com/mailru/easyjson: [b2ccc51 → v0.7.0](https://github.com/mailru/easyjson/compare/b2ccc51...v0.7.0)
- github.com/mattn/go-isatty: [v0.0.4 → v0.0.9](https://github.com/mattn/go-isatty/compare/v0.0.4...v0.0.9)
- github.com/munnerz/goautoneg: [a547fc6 → a7dc8b6](https://github.com/munnerz/goautoneg/compare/a547fc6...a7dc8b6)
- github.com/onsi/ginkgo: [v1.10.3 → v1.11.0](https://github.com/onsi/ginkgo/compare/v1.10.3...v1.11.0)
- github.com/prometheus/client_model: [14fe0d1 → v0.2.0](https://github.com/prometheus/client_model/compare/14fe0d1...v0.2.0)
- github.com/satori/go.uuid: [0aa62d5 → v1.2.0](https://github.com/satori/go.uuid/compare/0aa62d5...v1.2.0)
- github.com/spf13/jwalterweatherman: [v1.0.0 → v1.1.0](https://github.com/spf13/jwalterweatherman/compare/v1.0.0...v1.1.0)
- github.com/urfave/cli: [v1.18.0 → v1.20.0](https://github.com/urfave/cli/compare/v1.18.0...v1.20.0)
- github.com/xiang90/probing: [07dd2e8 → 43a291a](https://github.com/xiang90/probing/compare/07dd2e8...43a291a)
- go.etcd.io/bbolt: v1.3.1-etcd.7 → v1.3.3
- go.etcd.io/etcd: 83304cf → 3cf2f69
- go.uber.org/zap: v1.9.1 → v1.10.0
- golang.org/x/crypto: 5c40567 → bac4c82
- golang.org/x/tools: 5eefd05 → 6862ede
- golang.org/x/xerrors: a985d34 → 1b5146a
- gonum.org/v1/gonum: 3d26580 → v0.6.2
- google.golang.org/grpc: v1.25.1 → v1.27.1
- gopkg.in/natefinch/lumberjack.v2: 20b71e5 → v2.0.0
- gopkg.in/square/go-jose.v2: 89060de → v2.2.2
- gopkg.in/yaml.v2: v2.2.7 → v2.2.8
- honnef.co/go/tools: ea95bdf → v0.0.1-2020.1.3
- k8s.io/api: bd6ac52 → v0.18.0
- k8s.io/apiextensions-apiserver: 3de7581 → v0.18.0
- k8s.io/apimachinery: v0.17.1 → v0.18.0
- k8s.io/apiserver: 1e17798 → v0.18.0
- k8s.io/client-go: 6502b5e → v0.18.0
- k8s.io/code-generator: 732c9ca → v0.18.0
- k8s.io/component-base: ed2f086 → v0.18.0
- k8s.io/gengo: 26a6646 → 36b2048
- k8s.io/kube-openapi: 30be4d1 → bf4fb3b
- k8s.io/kubernetes: v1.14.7 → v1.18.0
- k8s.io/utils: 8619460 → a9aa75a
- mvdan.cc/xurls/v2: v2.0.0 → v2.1.0
- sigs.k8s.io/yaml: v1.1.0 → v1.2.0

### Removed
- github.com/coreos/bbolt: [v1.3.1-coreos.6](https://github.com/coreos/bbolt/tree/v1.3.1-coreos.6)
- github.com/natefinch/lumberjack: [v2.0.0+incompatible](https://github.com/natefinch/lumberjack/tree/v2.0.0)
- gopkg.in/yaml.v1: 9f9df34
- sigs.k8s.io/structured-merge-diff: 15d366b
