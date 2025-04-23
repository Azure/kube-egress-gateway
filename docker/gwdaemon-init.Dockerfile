# syntax=docker/dockerfile:1
FROM debian:latest
RUN apt-get update; apt-get install iproute2 -y
ENTRYPOINT ["/bin/sh", "-c", "ip netns exec ns-static-egress-gateway ip a || ip netns add ns-static-egress-gateway"]

