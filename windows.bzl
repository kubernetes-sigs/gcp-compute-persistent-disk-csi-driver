load("@io_bazel_rules_docker//container:container.bzl", "container_image", "container_push")

def windows_docker_push(base_version):
    container_image(
        name = "gce-pd-windows%s-image" % (base_version),
        base = ":servercore_%s" % (base_version),
        cmd = ["c:\\gce-pd-csi-driver-windows.exe"],
        entrypoint = ["c:\\gce-pd-csi-driver-windows.exe"],
        files = [":gce-pd-csi-driver-windows.exe"],
        operating_system = "windows",
        workdir = "c:\\",
        user = "ContainerAdministrator",
        env = { "CSI_PROXY_PIPE": "\\\\.\\pipe\\csipipe"},
    )
    container_push(
        name = "gce-pd-windows%s" % (base_version),
        image = "gce-pd-windows%s-image" % (base_version),
        format = "Docker",
        registry = "gcr.io",
        repository = "$(PROJECT_ID)/gce-pd-windows-%s" % (base_version),
        tag = "$(TAG)",
    )
