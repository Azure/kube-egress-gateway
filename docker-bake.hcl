
group "default" {
  targets = ["daemon", "controller"]
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

