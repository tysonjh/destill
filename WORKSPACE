workspace(name = "destill_agent")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "67a9d471ef8f4e8af60c18a3afc64d9f64cd4d5dcfb5aedc80e22b47f6f9bd1c",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.54.0/rules_go-v0.54.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.54.0/rules_go-v0.54.0.zip",
    ],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "3a5a1adb48eb8393b4b003e88b2bc95e0bb3d7aa19dd38e6d27ef0a2e2ed86ec",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.42.0/bazel-gazelle-v0.42.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.42.0/bazel-gazelle-v0.42.0.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(version = "1.23.4")

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

gazelle_dependencies()
