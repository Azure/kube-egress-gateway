
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

target "cnimanager" {
  inherits = ["base","cnimanager-tags"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-cnimanager",
  }
}
