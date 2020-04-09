load("@io_bazel_rules_docker//container:container.bzl", "container_image", "container_import", "container_push")
load("//:windows.bzl", "windows_docker_push")
container_import(
    name = "servercore_1909",
    config = "servercore.1909.config.json",
    layers = [],
    manifest = "servercore.1909.manifest.json",
)
container_import(
    name = "servercore_2019",
    config = "servercore.ltsc2019.config.json",
    layers = [],
    manifest = "servercore.ltsc2019.manifest.json",
)
# should be run with:
# bazel run --cpu=x86_64-windows --define PROJECT_ID=<your project id> --define TAG=dev //:gke-metrics-agent-windows1909
windows_docker_push(base_version = "2019")
#windows_docker_push(base_version = "1909")
