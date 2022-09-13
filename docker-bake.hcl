variable "TAG" {
  default = "dev"
}
variable "REPO" {
  default = "ghcr.io"
}

group "default" {
  targets = ["daemon", "controller"]
}

target "base" {
  dockerfile = "base.Dockerfile"
}
target "daemon" {
  tags = ["${REPO}/azure/kube-egress-gateway:daemon-${TAG}"]
  inherits = ["base"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-controller",
  }
}

target "controller" {
  inherits = ["base"]
  tags = ["${REPO}/azure/kube-egress-gateway:controller-${TAG}"]
  args = {
    MAIN_ENTRY = "kube-egress-gateway-daemon",
  }
}

