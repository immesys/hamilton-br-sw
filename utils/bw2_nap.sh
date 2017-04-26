#!/bin/bash
set -x
echo "BW2 NAP TRIGGER" >> /volatile/napp
journalctl --lines 1000 -u bw2 >> /volatile/napp
if [ $(cat /volatile/napp | grep -e "BW2 NAP TRIGGER" | wc -l ) -gt 10 ]
then
  echo "DOING HARD NAPP" | wall
  sleep 30
fi
