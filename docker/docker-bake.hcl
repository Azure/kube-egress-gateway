
group "default" {
  targets = ["daemon", "controller", "cnimanager", "cni", "cni-ipam", "daemoninit"]
}

variable "PLATFORMS" {
  default = "linux/amd64"
}

target "base" {
  dockerfile = "docker/base.Dockerfile"
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
  dockerfile = "docker/gwdaemon.Dockerfile"
  contexts = {
    baseimg = "target:daemon-compile"
  }
  platforms = [PLATFORMS]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-daemon",
  }
}

target "add-netns-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "add-netns",
  }
}

target "daemoninit" {
  inherits = ["daemoninit-tags"]
  dockerfile = "docker/gwdaemon-init.Dockerfile"
  contexts = {
    tool = "target:add-netns-compile"
  }
  platforms = [PLATFORMS]
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
  dockerfile = "docker/cnimanager.Dockerfile"
  contexts = {
    baseimg = "target:cnimanager-compile"
  }
  platforms = [PLATFORMS]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-cnimanager",
  }
}

target "cni-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "kube-egress-cni",
  }
}

target "copy-compile" {
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "copy",
  }
}

target "cni" {
  inherits = ["cni-tags"]
  dockerfile = "docker/cni.Dockerfile"
  contexts = {
    baseimg = "target:cni-compile",
    tool = "target:copy-compile"
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
  dockerfile = "docker/cni-ipam.Dockerfile"
  contexts = {
    baseimg = "target:cni-ipam-compile",
    tool = "target:copy-compile"
  }
  platforms = [PLATFORMS]
}