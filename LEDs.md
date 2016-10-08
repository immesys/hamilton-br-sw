# LED codes

The hamilton BR has five LEDs used to indicate the status of the border router.
This table explains what the codes mean.

## Overview

The TX led toggles whenever a packet is sent from the border router to the mesh.
The RX led toggles whenever a packet is received from the mesh at the border
router.

| LED | Status | Meaning           |
| --- | ------ | ----------------- |
| TX/RX  | Toggling | Packets are being sent from the BR to the mesh |
| TX/RX  | Solid on/off | No packets are being sent from the BR to the mesh |
| MCU | On | The NIC is active, and is receiving heartbeats from the Pi |
| MCU | Blinking | The NIC is active, but is not receiving heartbeats, the Pi may be faulty |
| MCU | Off | There is a critical problem with the MCU |
| PI | On | The Pi is active and receiving heartbeats from the MCU |
| PI | Blinking | The Pi is active, but is not receiving heartbeats, the MCU may be faulty |
| PI | Off | There is a critical problem with the Pi |
| WAN | On | The WAN link is healthy, and the Pi is receiving heartbeats from the Internet |
| WAN | Blinking (1 blink) | The WAN link is healthy, but the bosswave chain is synchronizing |
| WAN | Blinking (2 blinks) | The WAN link is healthy, but the bosswave permissions are invalid |
| WAN | Off | The WAN link is down, you may need to reconfigure the BR |
