
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
