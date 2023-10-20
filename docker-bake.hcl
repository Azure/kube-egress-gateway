
group "default" {
  targets = ["daemon", "controller", "cnimanager", "cni", "cni-ipam"]
}

variable "PLATFORMS" {
  default = "linux/amd64"
}

target "base" {
  dockerfile = "base.Dockerfile"
  platforms = [PLATFORMS]
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
  platforms = [PLATFORMS]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-daemon",
  }
}

target "controller" {
  inherits = ["base","controller-tags"]
  platforms = [PLATFORMS]
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
  platforms = [PLATFORMS]
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
  platforms = [PLATFORMS]
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
  platforms = [PLATFORMS]
}