#!/bin/bash

PREFIX=2001:410::/48
ip tuntap add tap0 mode tap
sysctl -w net.ipv6.conf.tap0.forwarding=1
sysctl -w net.ipv6.conf.tap0.accept_ra=0
ip link set tap0 up
ip a a fe80::1/64 dev tap0
ip a a fd00:dead:beef::1/128 dev lo
ip route add ${PREFIX} via fe80::2 dev tap0
