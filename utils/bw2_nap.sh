#!/bin/bash
set -x
umount /merged
umount /volatile
mkfs.ext4 -F /dev/sda2
reboot
