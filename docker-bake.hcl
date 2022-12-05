
group "default" {
  targets = ["daemon", "controller", "cnimanager"]
}

target "base" {
  dockerfile = "base.Dockerfile"
}

target "daemon" {
  inherits = ["base","daemon-tags"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-daemon",
    BASE_IMAGE = "mcr.microsoft.com/aks/devinfra/base-os-runtime-nettools:master.221105.1",
  }
}

target "controller" {
  inherits = ["base","controller-tags"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-controller",
  }
}

target "cnimanager-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-cnimanager",
  }
}

target "cnimanager" {
  inherits = ["cnimanager-tags"]
  dockerfile = "cnimanager.Dockerfile"
  contexts = {
    baseimg = "target:cnimanager-compile"
  }
  args = {
    GRPC_HEALTH_PROBE_VERSION = "v0.4.14"
  }
}