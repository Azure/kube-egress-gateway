
group "default" {
  targets = ["daemon", "controller", "cnimanager", "cni", "cni-ipam"]
}

target "base" {
  dockerfile = "base.Dockerfile"
}

target "daemon-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-daemon",
  }
}
target "daemon" {
  inherits = ["daemon-tags"]
  dockerfile = "gwdaemon.Dockerfile"
  contexts = {
    baseimg = "target:daemon-compile"
  }
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
    MAIN_ENTRY = "kube-egress-gateway-cnimanager",
    GRPC_HEALTH_PROBE_VERSION = "v0.4.14"
  }
}
target "cni-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "kube-egress-cni",
  }
}
target "cni" {
  inherits = ["cni-tags"]
  dockerfile = "cni.Dockerfile"
  contexts = {
    baseimg = "target:cni-compile"
  }
}

target "cni-ipam-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "kube-egress-cni-ipam",
  }
}
target "cni-ipam" {
  inherits = ["cni-ipam-tags"]
  dockerfile = "cni.Dockerfile"
  contexts = {
    baseimg = "target:cni-ipam-compile"
  }
}