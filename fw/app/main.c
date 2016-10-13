/*
 * Copyright (C) 2015 Freie Universität Berlin
 *
 * This file is subject to the terms and conditions of the GNU Lesser
 * General Public License v2.1. See the file LICENSE in the top level
 * directory for more details.
 */

/**
 * @ingroup     examples
 * @{
 *
 * @file
 * @brief       Example application for demonstrating the RIOT network stack
 *
 * @author      Hauke Petersen <hauke.petersen@fu-berlin.de>
 *
 * @}
 */

#include <stdio.h>
#include "xtimer.h"
#include "msg.h"
#include "periph/gpio.h"
#include "rethos.h"

#define MAIN_QUEUE_SIZE     (8)

#define D1_PIN GPIO_PIN(0, 27)
#define D2_PIN GPIO_PIN(1, 23)
#define D3_PIN GPIO_PIN(1, 22)
#define D5_PIN GPIO_PIN(0, 23)

#define TX_TOGGLE (PORT->Group[0].OUTTGL.reg = (1<<27))
#define RX_TOGGLE (PORT->Group[1].OUTTGL.reg = (1<<23))
extern ethos_t ethos;
static msg_t _main_msg_queue[MAIN_QUEUE_SIZE];
kernel_pid_t start_l7g(void);

#define CHANNEL_HEARTBEATS 4
#define CHANNEL_L7G 5

#define HB_TYPE_MCU_TO_PI 1

#define MAX_HB_TIME 1000000u

#define FULLOFF  1
#define FULLON  2
#define BLINKING1  3
#define BLINKING2  4
#define BLINKING3  5

#define BUILDVER 120
int wan_status;
int hb_status;
uint64_t last_hb;

void heartbeat_callback(ethos_t *dev, uint8_t channel, const uint8_t *data, uint16_t length)
{
  if (length >= 4) {
    last_hb = xtimer_now64();
    wan_status = data[1];
  }
}

typedef struct __attribute__((packed))
{
  uint32_t type;
  uint64_t uptime;
  uint32_t buildver;
  uint32_t rx_crc_fail;
  uint32_t rx_bytes;
  uint32_t rx_frames;
  uint32_t tx_frames;
  uint32_t tx_bytes;
  uint32_t tx_retries;
} heartbeat_t;

// X X X X X X X X X
// 1 0 0 0 0 0 0 0 0
// 1 0 1 0 0 0 0 0 0
// 1 0 1 0 1 0 0 0 0
int main(void)
{

    gpio_init(D1_PIN, GPIO_OUT);
    gpio_init(D2_PIN, GPIO_OUT);
    gpio_init(D3_PIN, GPIO_OUT);
    gpio_init(D5_PIN, GPIO_OUT);

    // gpio_set(D1_PIN);
    // gpio_set(D2_PIN);
    // gpio_set(D3_PIN);
    // gpio_set(D5_PIN);
    /* we need a message queue for the thread running the shell in order to
     * receive potentially fast incoming networking packets */
    msg_init_queue(_main_msg_queue, MAIN_QUEUE_SIZE);

    /* start shell */
    puts("All up, running the shell now");
    start_l7g();

    rethos_handler_t hb_h = {.channel = CHANNEL_HEARTBEATS, .cb = heartbeat_callback};
    rethos_register_handler(&ethos, &hb_h);
    heartbeat_t hb;
    int count = 0;
    while(1)
    {
      count++;
      int interval = count % 12;
      xtimer_usleep(100000U);

      if (xtimer_now64() - last_hb < MAX_HB_TIME) {
        //Heartbeats ok
        gpio_set(D3_PIN);
        switch(wan_status) {
          case BLINKING1:
            if (interval == 0) {
              gpio_set(D5_PIN);
            } else {
              gpio_clear(D5_PIN);
            }
            break;
          case BLINKING2:
            if (interval == 0 || interval == 2) {
              gpio_set(D5_PIN);
            } else {
              gpio_clear(D5_PIN);
            }
            break;
          case BLINKING3:
            if (interval == 0 || interval == 2 || interval == 4) {
              gpio_set(D5_PIN);
            } else {
              gpio_clear(D5_PIN);
            }
            break;
          case FULLON:
            gpio_set(D5_PIN);
            break;
          default:
            gpio_clear(D5_PIN);
            break;
        }
      } else {
        if (interval < 6) {
          gpio_set(D3_PIN);
        } else {
          gpio_clear(D3_PIN);
        }
        gpio_clear(D5_PIN);
      }
      if (count % 5 == 0)
      {
        hb.type = HB_TYPE_MCU_TO_PI;
        hb.uptime = xtimer_now64();
        hb.rx_crc_fail = ethos.stats_rx_cksum_fail;
        hb.rx_bytes = ethos.stats_rx_bytes;
        hb.rx_frames = ethos.stats_rx_frames;
        hb.tx_frames = ethos.stats_tx_frames;
        hb.tx_bytes = ethos.stats_tx_bytes;
        hb.tx_retries = ethos.stats_tx_retries;
        hb.buildver = BUILDVER;
        rethos_send_frame(&ethos, (uint8_t*)&hb, sizeof(hb), CHANNEL_HEARTBEATS, RETHOS_FRAME_TYPE_DATA);
      }
    }
    /* should be never reached */
    return 0;
}
