# v1.4.1 - Changelog since v1.4.0

## Changes by Kind

### Bug or Regression

- Fix #942 that can cause attacher to think a disk has been attached even if the attach failed. (#945, @mattcary)

### Uncategorized

- Add support for NVMe persistent disks (#946, @pwschuurman)
- Cherry-pick #930: Update golang version to 1.17.8 for building drivers. (#936, @pwschuurman)

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# v1.4.0 - Changelog since v1.3.4

## Changes by Kind

### Feature

- Updates Kubernetes dependencies to v1.22.0 ([#814](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/814), [@chrishenzie](https://github.com/chrishenzie))
- Add attach/detach back off ([#847](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/847), [@lizhuqi](https://github.com/lizhuqi))
- Enables volume cloning. ([#854](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/854), [@amacaskill](https://github.com/amacaskill))
- Use the most recent 1.3.4 image for prow rc master ([#864](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/864), [@saikat-royc](https://github.com/saikat-royc))
- Change to distroless base image ([#870](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/870), [@amacaskill](https://github.com/amacaskill))
- Turn on controller-publish-readonly flag and add validation in pd-csi driver for when readonly is on ([#869](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/869), [@leiyiz](https://github.com/leiyiz))

### Documentation

- Doc and image update for 1.3.4 release ([#855](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/855), [@saikat-royc](https://github.com/saikat-royc))
- Update release 1.2.4 CHANGELOG ([#862](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/862), [@saikat-royc](https://github.com/saikat-royc))

### Bug or Regression

### Other (Cleanup or Flake)

- Update debian base image to 1.9.0 ([#826](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/826), [@saikat-royc](https://github.com/saikat-royc))
- Updates the CSI sanity test suite to v4.2.0 ([#816](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/816), [@chrishenzie](https://github.com/chrishenzie))

### Uncategorized

- Update snapshot sidecar roles ([#857](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/857), [@saikat-royc](https://github.com/saikat-royc))

## Dependencies

### Added
- cloud.google.com/go/firestore: v1.1.0
- github.com/OneOfOne/xxhash: [v1.2.2](https://github.com/OneOfOne/xxhash/tree/v1.2.2)
- github.com/antihax/optional: [v1.0.0](https://github.com/antihax/optional/tree/v1.0.0)
- github.com/armon/go-metrics: [f0300d1](https://github.com/armon/go-metrics/tree/f0300d1)
- github.com/armon/go-radix: [7fddfc3](https://github.com/armon/go-radix/tree/7fddfc3)
- github.com/benbjohnson/clock: [v1.0.3](https://github.com/benbjohnson/clock/tree/v1.0.3)
- github.com/bits-and-blooms/bitset: [v1.2.0](https://github.com/bits-and-blooms/bitset/tree/v1.2.0)
- github.com/bketelsen/crypt: [5cbc8cc](https://github.com/bketelsen/crypt/tree/5cbc8cc)
- github.com/certifi/gocertifi: [2c3bb06](https://github.com/certifi/gocertifi/tree/2c3bb06)
- github.com/cespare/xxhash/v2: [v2.1.1](https://github.com/cespare/xxhash/v2/tree/v2.1.1)
- github.com/cespare/xxhash: [v1.1.0](https://github.com/cespare/xxhash/tree/v1.1.0)
- github.com/checkpoint-restore/go-criu/v5: [v5.0.0](https://github.com/checkpoint-restore/go-criu/v5/tree/v5.0.0)
- github.com/cockroachdb/errors: [v1.2.4](https://github.com/cockroachdb/errors/tree/v1.2.4)
- github.com/cockroachdb/logtags: [eb05cc2](https://github.com/cockroachdb/logtags/tree/eb05cc2)
- github.com/containerd/cgroups: [0dbf7f0](https://github.com/containerd/cgroups/tree/0dbf7f0)
- github.com/containerd/continuity: [aaeac12](https://github.com/containerd/continuity/tree/aaeac12)
- github.com/containerd/fifo: [a9fb20d](https://github.com/containerd/fifo/tree/a9fb20d)
- github.com/containerd/go-runc: [5a6d9f3](https://github.com/containerd/go-runc/tree/5a6d9f3)
- github.com/containerd/ttrpc: [v1.0.2](https://github.com/containerd/ttrpc/tree/v1.0.2)
- github.com/coredns/caddy: [v1.1.0](https://github.com/coredns/caddy/tree/v1.1.0)
- github.com/coreos/bbolt: [v1.3.2](https://github.com/coreos/bbolt/tree/v1.3.2)
- github.com/coreos/go-systemd/v22: [v22.3.2](https://github.com/coreos/go-systemd/v22/tree/v22.3.2)
- github.com/cpuguy83/go-md2man/v2: [v2.0.0](https://github.com/cpuguy83/go-md2man/v2/tree/v2.0.0)
- github.com/dgryski/go-sip13: [e10d5fe](https://github.com/dgryski/go-sip13/tree/e10d5fe)
- github.com/felixge/httpsnoop: [v1.0.1](https://github.com/felixge/httpsnoop/tree/v1.0.1)
- github.com/form3tech-oss/jwt-go: [v3.2.3+incompatible](https://github.com/form3tech-oss/jwt-go/tree/v3.2.3)
- github.com/frankban/quicktest: [v1.11.3](https://github.com/frankban/quicktest/tree/v1.11.3)
- github.com/fvbommel/sortorder: [v1.0.1](https://github.com/fvbommel/sortorder/tree/v1.0.1)
- github.com/getsentry/raven-go: [v0.2.0](https://github.com/getsentry/raven-go/tree/v0.2.0)
- github.com/go-errors/errors: [v1.0.1](https://github.com/go-errors/errors/tree/v1.0.1)
- github.com/go-kit/log: [v0.1.0](https://github.com/go-kit/log/tree/v0.1.0)
- github.com/godbus/dbus/v5: [v5.0.4](https://github.com/godbus/dbus/v5/tree/v5.0.4)
- github.com/gofrs/uuid: [v4.0.0+incompatible](https://github.com/gofrs/uuid/tree/v4.0.0)
- github.com/google/shlex: [e7afc7f](https://github.com/google/shlex/tree/e7afc7f)
- github.com/hashicorp/consul/api: [v1.1.0](https://github.com/hashicorp/consul/api/tree/v1.1.0)
- github.com/hashicorp/consul/sdk: [v0.1.1](https://github.com/hashicorp/consul/sdk/tree/v0.1.1)
- github.com/hashicorp/go-cleanhttp: [v0.5.1](https://github.com/hashicorp/go-cleanhttp/tree/v0.5.1)
- github.com/hashicorp/go-immutable-radix: [v1.0.0](https://github.com/hashicorp/go-immutable-radix/tree/v1.0.0)
- github.com/hashicorp/go-msgpack: [v0.5.3](https://github.com/hashicorp/go-msgpack/tree/v0.5.3)
- github.com/hashicorp/go-rootcerts: [v1.0.0](https://github.com/hashicorp/go-rootcerts/tree/v1.0.0)
- github.com/hashicorp/go-sockaddr: [v1.0.0](https://github.com/hashicorp/go-sockaddr/tree/v1.0.0)
- github.com/hashicorp/go-uuid: [v1.0.1](https://github.com/hashicorp/go-uuid/tree/v1.0.1)
- github.com/hashicorp/go.net: [v0.0.1](https://github.com/hashicorp/go.net/tree/v0.0.1)
- github.com/hashicorp/logutils: [v1.0.0](https://github.com/hashicorp/logutils/tree/v1.0.0)
- github.com/hashicorp/mdns: [v1.0.0](https://github.com/hashicorp/mdns/tree/v1.0.0)
- github.com/hashicorp/memberlist: [v0.1.3](https://github.com/hashicorp/memberlist/tree/v0.1.3)
- github.com/hashicorp/serf: [v0.8.2](https://github.com/hashicorp/serf/tree/v0.8.2)
- github.com/ishidawataru/sctp: [7c296d4](https://github.com/ishidawataru/sctp/tree/7c296d4)
- github.com/jmespath/go-jmespath/internal/testify: [v1.5.1](https://github.com/jmespath/go-jmespath/internal/testify/tree/v1.5.1)
- github.com/josharian/intern: [v1.0.0](https://github.com/josharian/intern/tree/v1.0.0)
- github.com/jpillora/backoff: [v1.0.0](https://github.com/jpillora/backoff/tree/v1.0.0)
- github.com/kubernetes-csi/csi-test/v4: [v4.2.0](https://github.com/kubernetes-csi/csi-test/v4/tree/v4.2.0)
- github.com/mitchellh/cli: [v1.0.0](https://github.com/mitchellh/cli/tree/v1.0.0)
- github.com/mitchellh/go-testing-interface: [v1.0.0](https://github.com/mitchellh/go-testing-interface/tree/v1.0.0)
- github.com/mitchellh/gox: [v0.4.0](https://github.com/mitchellh/gox/tree/v0.4.0)
- github.com/mitchellh/iochan: [v1.0.0](https://github.com/mitchellh/iochan/tree/v1.0.0)
- github.com/moby/ipvs: [v1.0.1](https://github.com/moby/ipvs/tree/v1.0.1)
- github.com/moby/spdystream: [v0.2.0](https://github.com/moby/spdystream/tree/v0.2.0)
- github.com/moby/sys/mountinfo: [v0.4.1](https://github.com/moby/sys/mountinfo/tree/v0.4.1)
- github.com/moby/term: [9d4ed18](https://github.com/moby/term/tree/9d4ed18)
- github.com/monochromegane/go-gitignore: [205db1a](https://github.com/monochromegane/go-gitignore/tree/205db1a)
- github.com/niemeyer/pretty: [a10e7ca](https://github.com/niemeyer/pretty/tree/a10e7ca)
- github.com/nxadm/tail: [v1.4.5](https://github.com/nxadm/tail/tree/v1.4.5)
- github.com/oklog/ulid: [v1.3.1](https://github.com/oklog/ulid/tree/v1.3.1)
- github.com/opentracing/opentracing-go: [v1.1.0](https://github.com/opentracing/opentracing-go/tree/v1.1.0)
- github.com/pascaldekloe/goe: [57f6aae](https://github.com/pascaldekloe/goe/tree/57f6aae)
- github.com/posener/complete: [v1.1.1](https://github.com/posener/complete/tree/v1.1.1)
- github.com/prometheus/tsdb: [v0.7.1](https://github.com/prometheus/tsdb/tree/v0.7.1)
- github.com/robertkrimen/otto: [ef014fd](https://github.com/robertkrimen/otto/tree/ef014fd)
- github.com/robfig/cron/v3: [v3.0.1](https://github.com/robfig/cron/v3/tree/v3.0.1)
- github.com/russross/blackfriday/v2: [v2.0.1](https://github.com/russross/blackfriday/v2/tree/v2.0.1)
- github.com/ryanuber/columnize: [9b3edd6](https://github.com/ryanuber/columnize/tree/9b3edd6)
- github.com/sean-/seed: [e2103e2](https://github.com/sean-/seed/tree/e2103e2)
- github.com/shurcooL/sanitized_anchor_name: [v1.0.0](https://github.com/shurcooL/sanitized_anchor_name/tree/v1.0.0)
- github.com/spaolacci/murmur3: [f09979e](https://github.com/spaolacci/murmur3/tree/f09979e)
- github.com/stoewer/go-strcase: [v1.2.0](https://github.com/stoewer/go-strcase/tree/v1.2.0)
- github.com/subosito/gotenv: [v1.2.0](https://github.com/subosito/gotenv/tree/v1.2.0)
- github.com/willf/bitset: [v1.1.11](https://github.com/willf/bitset/tree/v1.1.11)
- github.com/xlab/treeprint: [a009c39](https://github.com/xlab/treeprint/tree/a009c39)
- go.etcd.io/etcd/api/v3: v3.5.0
- go.etcd.io/etcd/client/pkg/v3: v3.5.0
- go.etcd.io/etcd/client/v2: v2.305.0
- go.etcd.io/etcd/client/v3: v3.5.0
- go.etcd.io/etcd/pkg/v3: v3.5.0
- go.etcd.io/etcd/raft/v3: v3.5.0
- go.etcd.io/etcd/server/v3: v3.5.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.20.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.20.0
- go.opentelemetry.io/contrib: v0.20.0
- go.opentelemetry.io/otel/exporters/otlp: v0.20.0
- go.opentelemetry.io/otel/metric: v0.20.0
- go.opentelemetry.io/otel/oteltest: v0.20.0
- go.opentelemetry.io/otel/sdk/export/metric: v0.20.0
- go.opentelemetry.io/otel/sdk/metric: v0.20.0
- go.opentelemetry.io/otel/sdk: v0.20.0
- go.opentelemetry.io/otel/trace: v0.20.0
- go.opentelemetry.io/otel: v0.20.0
- go.opentelemetry.io/proto/otlp: v0.7.0
- go.starlark.net: 8dd3e2e
- go.uber.org/goleak: v1.1.10
- golang.org/x/term: 6a3ed07
- gopkg.in/ini.v1: v1.51.0
- gopkg.in/sourcemap.v1: v1.0.5
- gotest.tools/v3: v3.0.3
- k8s.io/component-helpers: v0.22.0
- k8s.io/controller-manager: v0.22.0
- k8s.io/pod-security-admission: v0.22.0
- sigs.k8s.io/kustomize/api: v0.8.11
- sigs.k8s.io/kustomize/cmd/config: v0.9.13
- sigs.k8s.io/kustomize/kustomize/v4: v4.2.0
- sigs.k8s.io/kustomize/kyaml: v0.11.0
- sigs.k8s.io/structured-merge-diff/v4: v4.1.2

### Changed
- dmitri.shuralyov.com/gpu/mtl: 666a987 → 28db891
- github.com/Azure/azure-sdk-for-go: [v35.0.0+incompatible → v55.0.0+incompatible](https://github.com/Azure/azure-sdk-for-go/compare/v35.0.0...v55.0.0)
- github.com/Azure/go-ansiterm: [d6e3b33 → d185dfc](https://github.com/Azure/go-ansiterm/compare/d6e3b33...d185dfc)
- github.com/Azure/go-autorest/autorest/adal: [v0.5.0 → v0.9.13](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.5.0...v0.9.13)
- github.com/Azure/go-autorest/autorest/date: [v0.1.0 → v0.3.0](https://github.com/Azure/go-autorest/autorest/date/compare/v0.1.0...v0.3.0)
- github.com/Azure/go-autorest/autorest/mocks: [v0.2.0 → v0.4.1](https://github.com/Azure/go-autorest/autorest/mocks/compare/v0.2.0...v0.4.1)
- github.com/Azure/go-autorest/autorest/to: [v0.2.0 → v0.4.0](https://github.com/Azure/go-autorest/autorest/to/compare/v0.2.0...v0.4.0)
- github.com/Azure/go-autorest/autorest: [v0.9.0 → v0.11.18](https://github.com/Azure/go-autorest/autorest/compare/v0.9.0...v0.11.18)
- github.com/Azure/go-autorest/logger: [v0.1.0 → v0.2.1](https://github.com/Azure/go-autorest/logger/compare/v0.1.0...v0.2.1)
- github.com/Azure/go-autorest/tracing: [v0.5.0 → v0.6.0](https://github.com/Azure/go-autorest/tracing/compare/v0.5.0...v0.6.0)
- github.com/Azure/go-autorest: [v11.1.2+incompatible → v14.2.0+incompatible](https://github.com/Azure/go-autorest/compare/v11.1.2...v14.2.0)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [27a4ced → 7901bc8](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/27a4ced...7901bc8)
- github.com/Microsoft/hcsshim: [672e52e → 5eafd15](https://github.com/Microsoft/hcsshim/compare/672e52e...5eafd15)
- github.com/NYTimes/gziphandler: [56545f4 → v1.1.1](https://github.com/NYTimes/gziphandler/compare/56545f4...v1.1.1)
- github.com/alecthomas/template: [a0175ee → fb15b89](https://github.com/alecthomas/template/compare/a0175ee...fb15b89)
- github.com/alecthomas/units: [2efee85 → f65c72e](https://github.com/alecthomas/units/compare/2efee85...f65c72e)
- github.com/auth0/go-jwt-middleware: [5493cab → v1.0.1](https://github.com/auth0/go-jwt-middleware/compare/5493cab...v1.0.1)
- github.com/aws/aws-sdk-go: [v1.28.2 → v1.38.49](https://github.com/aws/aws-sdk-go/compare/v1.28.2...v1.38.49)
- github.com/cilium/ebpf: [95b36a5 → v0.6.2](https://github.com/cilium/ebpf/compare/95b36a5...v0.6.2)
- github.com/cncf/udpa/go: [269d4d4 → 5459f2c](https://github.com/cncf/udpa/go/compare/269d4d4...5459f2c)
- github.com/cockroachdb/datadriven: [80d97fb → bf6692d](https://github.com/cockroachdb/datadriven/compare/80d97fb...bf6692d)
- github.com/container-storage-interface/spec: [v1.2.0 → v1.5.0](https://github.com/container-storage-interface/spec/compare/v1.2.0...v1.5.0)
- github.com/containerd/console: [84eeaae → v1.0.2](https://github.com/containerd/console/compare/84eeaae...v1.0.2)
- github.com/containerd/containerd: [v1.0.2 → v1.4.4](https://github.com/containerd/containerd/compare/v1.0.2...v1.4.4)
- github.com/containerd/typeurl: [2a93cfd → v1.0.1](https://github.com/containerd/typeurl/compare/2a93cfd...v1.0.1)
- github.com/containernetworking/cni: [v0.7.1 → v0.8.1](https://github.com/containernetworking/cni/compare/v0.7.1...v0.8.1)
- github.com/coredns/corefile-migration: [v1.0.6 → v1.0.12](https://github.com/coredns/corefile-migration/compare/v1.0.6...v1.0.12)
- github.com/coreos/etcd: [v3.3.10+incompatible → v3.3.13+incompatible](https://github.com/coreos/etcd/compare/v3.3.10...v3.3.13)
- github.com/coreos/pkg: [97fdf19 → 399ea9e](https://github.com/coreos/pkg/compare/97fdf19...399ea9e)
- github.com/creack/pty: [v1.1.7 → v1.1.11](https://github.com/creack/pty/compare/v1.1.7...v1.1.11)
- github.com/docker/docker: [71cd53e → v20.10.2+incompatible](https://github.com/docker/docker/compare/71cd53e...v20.10.2)
- github.com/envoyproxy/go-control-plane: [v0.9.4 → 668b12f](https://github.com/envoyproxy/go-control-plane/compare/v0.9.4...668b12f)
- github.com/evanphx/json-patch: [v4.5.0+incompatible → v4.11.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.5.0...v4.11.0)
- github.com/fsnotify/fsnotify: [v1.4.7 → v1.4.9](https://github.com/fsnotify/fsnotify/compare/v1.4.7...v1.4.9)
- github.com/go-kit/kit: [v0.8.0 → v0.9.0](https://github.com/go-kit/kit/compare/v0.8.0...v0.9.0)
- github.com/go-logfmt/logfmt: [v0.3.0 → v0.5.0](https://github.com/go-logfmt/logfmt/compare/v0.3.0...v0.5.0)
- github.com/go-logr/logr: [v0.2.0 → v0.4.0](https://github.com/go-logr/logr/compare/v0.2.0...v0.4.0)
- github.com/go-openapi/jsonpointer: [v0.19.3 → v0.19.5](https://github.com/go-openapi/jsonpointer/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/jsonreference: [v0.19.3 → v0.19.5](https://github.com/go-openapi/jsonreference/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/swag: [v0.19.5 → v0.19.14](https://github.com/go-openapi/swag/compare/v0.19.5...v0.19.14)
- github.com/gogo/protobuf: [v1.3.1 → v1.3.2](https://github.com/gogo/protobuf/compare/v1.3.1...v1.3.2)
- github.com/golang/groupcache: [8c9f03a → 41bb18b](https://github.com/golang/groupcache/compare/8c9f03a...41bb18b)
- github.com/golang/protobuf: [v1.4.2 → v1.5.2](https://github.com/golang/protobuf/compare/v1.4.2...v1.5.2)
- github.com/google/btree: [v1.0.0 → v1.0.1](https://github.com/google/btree/compare/v1.0.0...v1.0.1)
- github.com/google/cadvisor: [v0.35.0 → v0.39.2](https://github.com/google/cadvisor/compare/v0.35.0...v0.39.2)
- github.com/google/go-cmp: [v0.5.2 → v0.5.5](https://github.com/google/go-cmp/compare/v0.5.2...v0.5.5)
- github.com/google/uuid: [v1.1.1 → v1.1.2](https://github.com/google/uuid/compare/v1.1.1...v1.1.2)
- github.com/googleapis/gnostic: [v0.3.1 → v0.5.5](https://github.com/googleapis/gnostic/compare/v0.3.1...v0.5.5)
- github.com/gopherjs/gopherjs: [0766667 → fce0ec3](https://github.com/gopherjs/gopherjs/compare/0766667...fce0ec3)
- github.com/gorilla/mux: [v1.7.0 → v1.8.0](https://github.com/gorilla/mux/compare/v1.7.0...v1.8.0)
- github.com/gorilla/websocket: [v1.4.0 → v1.4.2](https://github.com/gorilla/websocket/compare/v1.4.0...v1.4.2)
- github.com/grpc-ecosystem/go-grpc-middleware: [f849b54 → v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/compare/f849b54...v1.3.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.9.5 → v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.5...v1.16.0)
- github.com/heketi/heketi: [c2e2a4a → v10.3.0+incompatible](https://github.com/heketi/heketi/compare/c2e2a4a...v10.3.0)
- github.com/jmespath/go-jmespath: [c2b33e8 → v0.4.0](https://github.com/jmespath/go-jmespath/compare/c2b33e8...v0.4.0)
- github.com/jonboulle/clockwork: [v0.1.0 → v0.2.2](https://github.com/jonboulle/clockwork/compare/v0.1.0...v0.2.2)
- github.com/json-iterator/go: [v1.1.8 → v1.1.11](https://github.com/json-iterator/go/compare/v1.1.8...v1.1.11)
- github.com/julienschmidt/httprouter: [v1.2.0 → v1.3.0](https://github.com/julienschmidt/httprouter/compare/v1.2.0...v1.3.0)
- github.com/karrick/godirwalk: [v1.7.5 → v1.16.1](https://github.com/karrick/godirwalk/compare/v1.7.5...v1.16.1)
- github.com/kisielk/errcheck: [v1.2.0 → v1.5.0](https://github.com/kisielk/errcheck/compare/v1.2.0...v1.5.0)
- github.com/konsorten/go-windows-terminal-sequences: [v1.0.2 → v1.0.3](https://github.com/konsorten/go-windows-terminal-sequences/compare/v1.0.2...v1.0.3)
- github.com/kr/pretty: [v0.2.0 → v0.2.1](https://github.com/kr/pretty/compare/v0.2.0...v0.2.1)
- github.com/kr/text: [v0.1.0 → v0.2.0](https://github.com/kr/text/compare/v0.1.0...v0.2.0)
- github.com/mailru/easyjson: [v0.7.0 → v0.7.6](https://github.com/mailru/easyjson/compare/v0.7.0...v0.7.6)
- github.com/mattn/go-isatty: [v0.0.9 → v0.0.4](https://github.com/mattn/go-isatty/compare/v0.0.9...v0.0.4)
- github.com/mattn/go-runewidth: [v0.0.2 → v0.0.7](https://github.com/mattn/go-runewidth/compare/v0.0.2...v0.0.7)
- github.com/matttproud/golang_protobuf_extensions: [v1.0.1 → c182aff](https://github.com/matttproud/golang_protobuf_extensions/compare/v1.0.1...c182aff)
- github.com/miekg/dns: [v1.1.4 → v1.0.14](https://github.com/miekg/dns/compare/v1.1.4...v1.0.14)
- github.com/mistifyio/go-zfs: [v2.1.1+incompatible → f784269](https://github.com/mistifyio/go-zfs/compare/v2.1.1...f784269)
- github.com/mrunalp/fileutils: [7d4729f → v0.5.0](https://github.com/mrunalp/fileutils/compare/7d4729f...v0.5.0)
- github.com/mwitkow/go-conntrack: [cc309e4 → 2f06839](https://github.com/mwitkow/go-conntrack/compare/cc309e4...2f06839)
- github.com/olekukonko/tablewriter: [a0225b3 → v0.0.4](https://github.com/olekukonko/tablewriter/compare/a0225b3...v0.0.4)
- github.com/onsi/ginkgo: [v1.11.0 → v1.14.2](https://github.com/onsi/ginkgo/compare/v1.11.0...v1.14.2)
- github.com/onsi/gomega: [v1.7.1 → v1.10.4](https://github.com/onsi/gomega/compare/v1.7.1...v1.10.4)
- github.com/opencontainers/go-digest: [v1.0.0-rc1 → v1.0.0](https://github.com/opencontainers/go-digest/compare/v1.0.0-rc1...v1.0.0)
- github.com/opencontainers/runc: [v1.0.0-rc10 → v1.0.1](https://github.com/opencontainers/runc/compare/v1.0.0-rc10...v1.0.1)
- github.com/opencontainers/runtime-spec: [v1.0.0 → 1c3f411](https://github.com/opencontainers/runtime-spec/compare/v1.0.0...1c3f411)
- github.com/opencontainers/selinux: [5215b18 → v1.8.2](https://github.com/opencontainers/selinux/compare/5215b18...v1.8.2)
- github.com/prometheus/client_golang: [v1.0.0 → v1.11.0](https://github.com/prometheus/client_golang/compare/v1.0.0...v1.11.0)
- github.com/prometheus/common: [v0.4.1 → v0.26.0](https://github.com/prometheus/common/compare/v0.4.1...v0.26.0)
- github.com/prometheus/procfs: [v0.0.8 → v0.6.0](https://github.com/prometheus/procfs/compare/v0.0.8...v0.6.0)
- github.com/quobyte/api: [v0.1.2 → v0.1.8](https://github.com/quobyte/api/compare/v0.1.2...v0.1.8)
- github.com/rogpeppe/fastuuid: [6724a57 → v1.2.0](https://github.com/rogpeppe/fastuuid/compare/6724a57...v1.2.0)
- github.com/rubiojr/go-vhd: [0bfd3b3 → 02e2102](https://github.com/rubiojr/go-vhd/compare/0bfd3b3...02e2102)
- github.com/satori/go.uuid: [v1.2.0 → 0aa62d5](https://github.com/satori/go.uuid/compare/v1.2.0...0aa62d5)
- github.com/sergi/go-diff: [v1.0.0 → v1.1.0](https://github.com/sergi/go-diff/compare/v1.0.0...v1.1.0)
- github.com/sirupsen/logrus: [v1.4.2 → v1.8.1](https://github.com/sirupsen/logrus/compare/v1.4.2...v1.8.1)
- github.com/smartystreets/assertions: [b2de0cb → v1.1.0](https://github.com/smartystreets/assertions/compare/b2de0cb...v1.1.0)
- github.com/soheilhy/cmux: [v0.1.4 → v0.1.5](https://github.com/soheilhy/cmux/compare/v0.1.4...v0.1.5)
- github.com/spf13/cobra: [v0.0.5 → v1.1.3](https://github.com/spf13/cobra/compare/v0.0.5...v1.1.3)
- github.com/spf13/jwalterweatherman: [v1.1.0 → v1.0.0](https://github.com/spf13/jwalterweatherman/compare/v1.1.0...v1.0.0)
- github.com/spf13/viper: [v1.3.2 → v1.7.0](https://github.com/spf13/viper/compare/v1.3.2...v1.7.0)
- github.com/storageos/go-api: [343b3ef → v2.2.0+incompatible](https://github.com/storageos/go-api/compare/343b3ef...v2.2.0)
- github.com/stretchr/testify: [v1.6.1 → v1.7.0](https://github.com/stretchr/testify/compare/v1.6.1...v1.7.0)
- github.com/syndtr/gocapability: [d983527 → 42c35b4](https://github.com/syndtr/gocapability/compare/d983527...42c35b4)
- github.com/tmc/grpc-websocket-proxy: [89b8d40 → e5319fd](https://github.com/tmc/grpc-websocket-proxy/compare/89b8d40...e5319fd)
- github.com/ugorji/go: [v1.1.1 → v1.1.4](https://github.com/ugorji/go/compare/v1.1.1...v1.1.4)
- github.com/urfave/cli: [v1.20.0 → v1.22.2](https://github.com/urfave/cli/compare/v1.20.0...v1.22.2)
- github.com/vishvananda/netlink: [v1.0.0 → v1.1.0](https://github.com/vishvananda/netlink/compare/v1.0.0...v1.1.0)
- github.com/vishvananda/netns: [be1fbed → db3c7e5](https://github.com/vishvananda/netns/compare/be1fbed...db3c7e5)
- github.com/yuin/goldmark: [v1.2.1 → v1.3.5](https://github.com/yuin/goldmark/compare/v1.2.1...v1.3.5)
- go.etcd.io/bbolt: v1.3.3 → v1.3.6
- go.etcd.io/etcd: 3cf2f69 → 83304cf
- go.uber.org/atomic: v1.3.2 → v1.7.0
- go.uber.org/multierr: v1.1.0 → v1.6.0
- go.uber.org/zap: v1.10.0 → v1.17.0
- golang.org/x/crypto: 75b2880 → 5ea612d
- golang.org/x/exp: 6cc2880 → 85be41e
- golang.org/x/lint: 738671d → 6edffad
- golang.org/x/mobile: d2bd2a2 → e6ae53a
- golang.org/x/mod: v0.3.0 → v0.4.2
- golang.org/x/net: f585440 → 37e1c6a
- golang.org/x/sync: 6e8e738 → 036812b
- golang.org/x/sys: fdedc70 → 59db8d7
- golang.org/x/text: v0.3.3 → v0.3.6
- golang.org/x/time: 555d28b → 1f47c86
- golang.org/x/tools: 39188db → v0.1.2
- google.golang.org/genproto: 0bd0a95 → f16073e
- google.golang.org/grpc: v1.31.1 → v1.38.0
- google.golang.org/protobuf: v1.25.0 → v1.26.0
- gopkg.in/check.v1: 41f04d3 → 8fa4692
- gopkg.in/yaml.v2: v2.2.8 → v2.4.0
- gopkg.in/yaml.v3: 9f266ea → 496545a
- k8s.io/api: v0.18.0 → v0.22.0
- k8s.io/apiextensions-apiserver: v0.18.0 → v0.22.0
- k8s.io/apimachinery: v0.18.0 → v0.22.0
- k8s.io/apiserver: v0.18.0 → v0.22.0
- k8s.io/cli-runtime: v0.18.0 → v0.22.0
- k8s.io/client-go: v0.18.0 → v0.22.0
- k8s.io/cloud-provider: v0.18.0 → v0.22.0
- k8s.io/cluster-bootstrap: v0.18.0 → v0.22.0
- k8s.io/code-generator: v0.18.0 → v0.22.0
- k8s.io/component-base: v0.18.0 → v0.22.0
- k8s.io/cri-api: v0.18.0 → v0.22.0
- k8s.io/csi-translation-lib: v0.18.0 → v0.22.0
- k8s.io/gengo: 36b2048 → b6c5ce2
- k8s.io/klog/v2: v2.4.0 → v2.9.0
- k8s.io/kube-aggregator: v0.18.0 → v0.22.0
- k8s.io/kube-controller-manager: v0.18.0 → v0.22.0
- k8s.io/kube-openapi: bf4fb3b → 9528897
- k8s.io/kube-proxy: v0.18.0 → v0.22.0
- k8s.io/kube-scheduler: v0.18.0 → v0.22.0
- k8s.io/kubectl: v0.18.0 → v0.22.0
- k8s.io/kubelet: v0.18.0 → v0.22.0
- k8s.io/kubernetes: v1.18.0 → v1.22.0
- k8s.io/legacy-cloud-providers: v0.18.0 → v0.22.0
- k8s.io/metrics: v0.18.0 → v0.22.0
- k8s.io/mount-utils: v0.20.6 → v0.22.0
- k8s.io/sample-apiserver: v0.18.0 → v0.22.0
- k8s.io/system-validators: v1.0.4 → v1.5.0
- k8s.io/utils: 67b214c → 4b05e18
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.7 → v0.0.22

### Removed
- github.com/OpenPeeDeeP/depguard: [v1.0.1](https://github.com/OpenPeeDeeP/depguard/tree/v1.0.1)
- github.com/Rican7/retry: [v0.1.0](https://github.com/Rican7/retry/tree/v0.1.0)
- github.com/StackExchange/wmi: [5d04971](https://github.com/StackExchange/wmi/tree/5d04971)
- github.com/agnivade/levenshtein: [v1.0.1](https://github.com/agnivade/levenshtein/tree/v1.0.1)
- github.com/andreyvit/diff: [c7f18ee](https://github.com/andreyvit/diff/tree/c7f18ee)
- github.com/anmitsu/go-shlex: [648efa6](https://github.com/anmitsu/go-shlex/tree/648efa6)
- github.com/bazelbuild/bazel-gazelle: [70208cb](https://github.com/bazelbuild/bazel-gazelle/tree/70208cb)
- github.com/bazelbuild/rules_go: [6dae44d](https://github.com/bazelbuild/rules_go/tree/6dae44d)
- github.com/bifurcation/mint: [93c51c6](https://github.com/bifurcation/mint/tree/93c51c6)
- github.com/bradfitz/go-smtpd: [deb6d62](https://github.com/bradfitz/go-smtpd/tree/deb6d62)
- github.com/caddyserver/caddy: [v1.0.3](https://github.com/caddyserver/caddy/tree/v1.0.3)
- github.com/cenkalti/backoff: [v2.1.1+incompatible](https://github.com/cenkalti/backoff/tree/v2.1.1)
- github.com/cespare/prettybench: [03b8cfe](https://github.com/cespare/prettybench/tree/03b8cfe)
- github.com/checkpoint-restore/go-criu: [17b0214](https://github.com/checkpoint-restore/go-criu/tree/17b0214)
- github.com/cheekybits/genny: [9127e81](https://github.com/cheekybits/genny/tree/9127e81)
- github.com/codegangsta/negroni: [v1.0.0](https://github.com/codegangsta/negroni/tree/v1.0.0)
- github.com/docker/libnetwork: [c8a5fca](https://github.com/docker/libnetwork/tree/c8a5fca)
- github.com/docker/spdystream: [449fdfc](https://github.com/docker/spdystream/tree/449fdfc)
- github.com/gliderlabs/ssh: [v0.1.1](https://github.com/gliderlabs/ssh/tree/v0.1.1)
- github.com/globalsign/mgo: [eeefdec](https://github.com/globalsign/mgo/tree/eeefdec)
- github.com/go-acme/lego: [v2.5.0+incompatible](https://github.com/go-acme/lego/tree/v2.5.0)
- github.com/go-bindata/go-bindata: [v3.1.1+incompatible](https://github.com/go-bindata/go-bindata/tree/v3.1.1)
- github.com/go-critic/go-critic: [1df3008](https://github.com/go-critic/go-critic/tree/1df3008)
- github.com/go-lintpack/lintpack: [v0.5.2](https://github.com/go-lintpack/lintpack/tree/v0.5.2)
- github.com/go-ole/go-ole: [v1.2.1](https://github.com/go-ole/go-ole/tree/v1.2.1)
- github.com/go-openapi/analysis: [v0.19.5](https://github.com/go-openapi/analysis/tree/v0.19.5)
- github.com/go-openapi/errors: [v0.19.2](https://github.com/go-openapi/errors/tree/v0.19.2)
- github.com/go-openapi/loads: [v0.19.4](https://github.com/go-openapi/loads/tree/v0.19.4)
- github.com/go-openapi/runtime: [v0.19.4](https://github.com/go-openapi/runtime/tree/v0.19.4)
- github.com/go-openapi/strfmt: [v0.19.3](https://github.com/go-openapi/strfmt/tree/v0.19.3)
- github.com/go-openapi/validate: [v0.19.5](https://github.com/go-openapi/validate/tree/v0.19.5)
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
- github.com/gostaticanalysis/analysisutil: [v0.0.3](https://github.com/gostaticanalysis/analysisutil/tree/v0.0.3)
- github.com/jellevandenhooff/dkim: [f50fe3d](https://github.com/jellevandenhooff/dkim/tree/f50fe3d)
- github.com/jimstudt/http-authentication: [3eca13d](https://github.com/jimstudt/http-authentication/tree/3eca13d)
- github.com/kubernetes-csi/csi-test/v3: [v3.0.0](https://github.com/kubernetes-csi/csi-test/v3/tree/v3.0.0)
- github.com/kylelemons/godebug: [d65d576](https://github.com/kylelemons/godebug/tree/d65d576)
- github.com/logrusorgru/aurora: [a7b3b31](https://github.com/logrusorgru/aurora/tree/a7b3b31)
- github.com/lucas-clemente/aes12: [cd47fb3](https://github.com/lucas-clemente/aes12/tree/cd47fb3)
- github.com/lucas-clemente/quic-clients: [v0.1.0](https://github.com/lucas-clemente/quic-clients/tree/v0.1.0)
- github.com/lucas-clemente/quic-go-certificates: [d2f8652](https://github.com/lucas-clemente/quic-go-certificates/tree/d2f8652)
- github.com/lucas-clemente/quic-go: [v0.10.2](https://github.com/lucas-clemente/quic-go/tree/v0.10.2)
- github.com/marten-seemann/qtls: [v0.2.3](https://github.com/marten-seemann/qtls/tree/v0.2.3)
- github.com/mattn/go-shellwords: [v1.0.5](https://github.com/mattn/go-shellwords/tree/v1.0.5)
- github.com/mattn/goveralls: [v0.0.2](https://github.com/mattn/goveralls/tree/v0.0.2)
- github.com/mesos/mesos-go: [v0.0.9](https://github.com/mesos/mesos-go/tree/v0.0.9)
- github.com/mholt/certmagic: [6a42ef9](https://github.com/mholt/certmagic/tree/6a42ef9)
- github.com/mitchellh/go-ps: [4fdf99a](https://github.com/mitchellh/go-ps/tree/4fdf99a)
- github.com/mozilla/tls-observatory: [8791a20](https://github.com/mozilla/tls-observatory/tree/8791a20)
- github.com/naoina/go-stringutil: [v0.1.0](https://github.com/naoina/go-stringutil/tree/v0.1.0)
- github.com/naoina/toml: [v0.1.1](https://github.com/naoina/toml/tree/v0.1.1)
- github.com/nbutton23/zxcvbn-go: [eafdab6](https://github.com/nbutton23/zxcvbn-go/tree/eafdab6)
- github.com/pborman/uuid: [v1.2.0](https://github.com/pborman/uuid/tree/v1.2.0)
- github.com/pquerna/ffjson: [af8b230](https://github.com/pquerna/ffjson/tree/af8b230)
- github.com/quasilyte/go-consistent: [c6f3937](https://github.com/quasilyte/go-consistent/tree/c6f3937)
- github.com/robfig/cron: [v1.1.0](https://github.com/robfig/cron/tree/v1.1.0)
- github.com/ryanuber/go-glob: [256dc44](https://github.com/ryanuber/go-glob/tree/256dc44)
- github.com/shirou/gopsutil: [c95755e](https://github.com/shirou/gopsutil/tree/c95755e)
- github.com/shirou/w32: [bb4de01](https://github.com/shirou/w32/tree/bb4de01)
- github.com/shurcooL/go-goon: [37c2f52](https://github.com/shurcooL/go-goon/tree/37c2f52)
- github.com/sourcegraph/go-diff: [v0.5.1](https://github.com/sourcegraph/go-diff/tree/v0.5.1)
- github.com/tarm/serial: [98f6abe](https://github.com/tarm/serial/tree/98f6abe)
- github.com/thecodeteam/goscaleio: [v0.1.0](https://github.com/thecodeteam/goscaleio/tree/v0.1.0)
- github.com/tidwall/pretty: [v1.0.0](https://github.com/tidwall/pretty/tree/v1.0.0)
- github.com/timakin/bodyclose: [87058b9](https://github.com/timakin/bodyclose/tree/87058b9)
- github.com/ultraware/funlen: [v0.0.2](https://github.com/ultraware/funlen/tree/v0.0.2)
- github.com/valyala/bytebufferpool: [v1.0.0](https://github.com/valyala/bytebufferpool/tree/v1.0.0)
- github.com/valyala/fasthttp: [v1.2.0](https://github.com/valyala/fasthttp/tree/v1.2.0)
- github.com/valyala/quicktemplate: [v1.1.1](https://github.com/valyala/quicktemplate/tree/v1.1.1)
- github.com/valyala/tcplisten: [ceec8f9](https://github.com/valyala/tcplisten/tree/ceec8f9)
- github.com/vektah/gqlparser: [v1.1.2](https://github.com/vektah/gqlparser/tree/v1.1.2)
- go.mongodb.org/mongo-driver: v1.1.2
- go4.org: 417644f
- golang.org/x/build: 2835ba2
- golang.org/x/perf: 6e6d33e
- gopkg.in/mcuadros/go-syslog.v2: v2.2.1
- gotest.tools/gotestsum: v0.3.5
- grpc.go4.org: 11d0a25
- k8s.io/heapster: v1.2.0-beta.1
- k8s.io/repo-infra: v0.0.1-alpha.1
- mvdan.cc/interfacer: c200402
- mvdan.cc/lint: adc824a
- mvdan.cc/unparam: fbb5962
- sigs.k8s.io/kustomize: v2.0.3+incompatible
- sigs.k8s.io/structured-merge-diff/v3: v3.0.0
- sourcegraph.com/sqs/pbtypes: d3ebe8f
