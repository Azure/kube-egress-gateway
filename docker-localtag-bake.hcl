variable "TAG" {
  default = "latest"
}

variable "IMAGE_REGISTRY" {
    default = "local"
}

target "daemon-tags" {
    tags = ["${IMAGE_REGISTRY}/kube-egress-gateway-daemon:${TAG}"]
}

target "controller-tags" {
    tags = ["${IMAGE_REGISTRY}/kube-egress-gateway-controller:${TAG}"]
}

target "cnimanager-tags" {
    tags = ["${IMAGE_REGISTRY}/kube-egress-gateway-cnimanager:${TAG}"]
}
