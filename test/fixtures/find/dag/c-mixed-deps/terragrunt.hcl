# Uses both dependency and dependencies blocks
dependency "single_dep" {
    config_path = "../a-dependent"
}

dependencies {
    paths = ["../d-dependencies-only"]
}
