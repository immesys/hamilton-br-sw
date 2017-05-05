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
#include "net/gnrc.h"
#include "net/gnrc/ipv6.h"
#include "net/gnrc/udp.h"
#include "net/gnrc/netif/hdr.h"
#include "net/gnrc/ipv6/netif.h"
#include "net/gnrc/ipv6/autoconf_onehop.h"

#include "cmsis/samr21/include/component/gclk.h"
#include "cmsis/samr21/include/component/wdt.h"
#include "cmsis/samr21/include/instance/wdt.h"

#define PAC0_BASE 0x40000000
#define WDT_BASE 0x40001000

void wdt_clear(void) {
    volatile uint8_t* wdt_status = (volatile uint8_t*) 0x40001007;
    volatile uint8_t* wdt_clear = (volatile uint8_t*) 0x40001008;

    while (*wdt_status);
    *wdt_clear = 0xA5;
    while (*wdt_status);
}

void wdt_setup(void) {
    /* Enable the bus for WDT and PAC0. */
    volatile uint32_t* pm_apbamask = (volatile uint32_t*) 0x40000418;
    *pm_apbamask = 0x0000007f;

    /* Setup GCLK_WDT at (32 kHz) / (2 ^ (7 + 1)) = 128 Hz. */
    GCLK->GENDIV.reg  = (GCLK_GENDIV_ID(3)  | GCLK_GENDIV_DIV(7));
    GCLK->GENCTRL.reg = (GCLK_GENCTRL_ID(3) | GCLK_GENCTRL_GENEN | GCLK_GENCTRL_DIVSEL | GCLK_GENCTRL_RUNSTDBY | GCLK_GENCTRL_SRC_OSCULP32K);
    while (GCLK->STATUS.bit.SYNCBUSY);

    GCLK->CLKCTRL.reg = (GCLK_CLKCTRL_GEN(3) | GCLK_CLKCTRL_CLKEN | GCLK_CLKCTRL_ID(GCLK_CLKCTRL_ID_WDT_Val));
    while (GCLK->STATUS.bit.SYNCBUSY);

    /* Set up the WDT to reset after 16384 cycles (128 s), if not cleared. */
    volatile uint8_t* wdt_status = (volatile uint8_t*) 0x40001007;
    volatile uint8_t* wdt_config = (volatile uint8_t*) 0x40001001;
    volatile uint8_t* wdt_ctrl = (volatile uint8_t*) 0x40001000;

    while (*wdt_status);
    *wdt_config = 0x0B;
    while (*wdt_status);
    *wdt_ctrl = 0x02;
    while (*wdt_status);
}

#define MAIN_QUEUE_SIZE     (8)

#define D1_PIN GPIO_PIN(0, 27)
#define D2_PIN GPIO_PIN(1, 23)
#define D3_PIN GPIO_PIN(1, 22)
#define D5_PIN GPIO_PIN(0, 23)

#define TX_TOGGLE (PORT->Group[0].OUTTGL.reg = (1<<27))
#define RX_TOGGLE (PORT->Group[1].OUTTGL.reg = (1<<23))
extern ethos_t rethos;
static msg_t _main_msg_queue[MAIN_QUEUE_SIZE];

kernel_pid_t start_br(void);
kernel_pid_t start_l7g(void);

#define CHANNEL_HEARTBEATS 4

#define HB_TYPE_MCU_TO_PI 1

#define MAX_HB_TIME 1000000u

#define FULLOFF  1
#define FULLON  2
#define BLINKING1  3
#define BLINKING2  4
#define BLINKING3  5

#define BUILDVER 200
int wan_status;
int hb_status;
uint64_t last_hb;

#define CHANNEL_DOWNLINK 7

ipv6_addr_t ipv6_addr;
const uint8_t ipv6_prefix_bytes = 8;

void heartbeat_callback(ethos_t *dev, uint8_t channel, uint8_t *data, uint16_t length)
{
    if (length >= 4) {
        last_hb = xtimer_now_usec64();
        wan_status = data[1];
    }
}

void downlink_callback(ethos_t* dev, uint8_t channel, uint8_t* data, uint16_t length)
{
    gnrc_pktsnip_t* pkt = gnrc_pktbuf_add(NULL, data, length, GNRC_NETTYPE_IPV6);
    if (pkt == NULL) {
        return;
    }
    if (gnrc_netapi_dispatch_receive(GNRC_NETTYPE_IPV6, GNRC_NETREG_DEMUX_CTX_ALL, pkt) == 0) {
        gnrc_pktbuf_release(pkt);
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

heartbeat_t hb;

int get_ipv6_addr_from_ll(ipv6_addr_t* my_addr, kernel_pid_t radio_pid) {
    ipv6_addr_t my_ipv6_addr;
    if (ipv6_addr_from_str(&my_ipv6_addr, HAMILTON_BORDER_ROUTER_ADDRESS) == NULL) {
        perror("invalid HAMILTON_BORDER_ROUTER_ADDRESS");
        return 1;
    }

    eui64_t my_ll_addr;
    gnrc_netapi_opt_t addr_req_opt;
    msg_t addr_req;
    msg_t addr_resp;

    addr_req.type = GNRC_NETAPI_MSG_TYPE_GET;
    addr_req.content.ptr = &addr_req_opt;

    addr_req_opt.opt = NETOPT_ADDRESS_LONG;
    addr_req_opt.data = &my_ll_addr;
    addr_req_opt.data_len = sizeof(eui64_t);

    msg_send_receive(&addr_req, &addr_resp, radio_pid);

    if (addr_resp.content.value != 8) {
        printf("Link layer address length is not 8 bytes (got %u)\n", (unsigned int) addr_resp.content.value);
        return 1;
    }

    if (gnrc_ipv6_autoconf_l2addr_to_ipv6(&my_ipv6_addr, &my_ll_addr) != 0) {
        printf("Could not convert link-layer address to IP address\n");
        return 1;
    }

    if (my_addr != NULL) {
        memcpy(my_addr, &my_ipv6_addr, sizeof(ipv6_addr_t));
    }

    return 0;
}

// X X X X X X X X X
// 1 0 0 0 0 0 0 0 0
// 1 0 1 0 0 0 0 0 0
// 1 0 1 0 1 0 0 0 0
int main(void)
{
    /* Set up the watchdog before anything else. */
    wdt_setup();

    kernel_pid_t radio_pid = get_6lowpan_pid();
    assert(radio_pid != 0);

    if (get_ipv6_addr_from_ll(&ipv6_addr, radio_pid) != 0) {
        printf("Could not set IPv6 address from link layer\n");
        return 1;
    }

    char ipbuf[IPV6_ADDR_MAX_STR_LEN + 1];
    char* ipstr = ipv6_addr_to_str(ipbuf, &ipv6_addr, sizeof(ipbuf));
    if (ipstr != ipbuf) {
        perror("inet_ntop");
        return 1;
    }

    printf("My IP address is %s\n", ipstr);

    //gnrc_ipv6_netif_t* radio_if = gnrc_ipv6_netif_get(radio_pid);
    //assert(radio_if != NULL);
    gnrc_ipv6_netif_add_addr(radio_pid, &ipv6_addr, ipv6_prefix_bytes << 3, 0);
    //gnrc_ipv6_netif_set_router(radio_if, true);

    gpio_init(D1_PIN, GPIO_OUT);
    gpio_init(D2_PIN, GPIO_OUT);
    gpio_init(D3_PIN, GPIO_OUT);
    gpio_init(D5_PIN, GPIO_OUT);

    /* we need a message queue for the thread running the shell in order to
     * receive potentially fast incoming networking packets */
    msg_init_queue(_main_msg_queue, MAIN_QUEUE_SIZE);

    /* start shell */
    puts("All up, running the border router now");
    start_br();
    start_l7g();

    rethos_handler_t hb_h = {.channel = CHANNEL_HEARTBEATS, .cb = heartbeat_callback};
    rethos_register_handler(&rethos, &hb_h);
    rethos_handler_t pkt_h = {.channel = CHANNEL_DOWNLINK, .cb = downlink_callback};
    rethos_register_handler(&rethos, &pkt_h);
    int count = 0;
    while(1)
    {
      count++;
      int interval = count % 12;
      xtimer_usleep(100000U);

      if (xtimer_now_usec64() - last_hb < MAX_HB_TIME) {
        //Heartbeats ok
        wdt_clear();
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
      if (count % 5 == 0) {
        hb.type = HB_TYPE_MCU_TO_PI;
        hb.uptime = xtimer_now_usec64();
        hb.rx_crc_fail = rethos.stats_rx_cksum_fail;
        hb.rx_bytes = rethos.stats_rx_bytes;
        hb.rx_frames = rethos.stats_rx_frames;
        hb.tx_frames = rethos.stats_tx_frames;
        hb.tx_bytes = rethos.stats_tx_bytes;
        hb.tx_retries = rethos.stats_tx_retries;
        hb.buildver = BUILDVER;
        rethos_send_frame(&rethos, (uint8_t*) &hb, sizeof(hb), CHANNEL_HEARTBEATS, RETHOS_FRAME_TYPE_DATA);
      }
    }
    /* should be never reached */
    return 0;
}
