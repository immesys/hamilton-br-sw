
#include <stdio.h>
#include <inttypes.h>

#include "net/gnrc.h"
#include "net/gnrc/ipv6.h"
#include "net/gnrc/udp.h"
#include "net/gnrc/netif/hdr.h"
#include <checksum/fletcher16.h>
#include <rethos.h>
#include <board.h>

#define CHANNEL_L7G 5

extern ethos_t rethos;

#define Q_SZ 256

typedef struct __attribute__((packed))
{
  uint16_t len;
  uint8_t srcmac[8];
  uint8_t srcip[16];
  uint64_t recv_time;
  uint8_t rssi;
  uint8_t lqi;
} staging_hdr_t;

static staging_hdr_t shdr;
void _handle_incoming_pkt(gnrc_pktsnip_t *p)
{
  gnrc_pktsnip_t *tmp;
  LL_SEARCH_SCALAR(p, tmp, type, GNRC_NETTYPE_IPV6);
  ipv6_hdr_t *ip = (ipv6_hdr_t *)tmp->data;
  LL_SEARCH_SCALAR(p, tmp, type, GNRC_NETTYPE_NETIF);
  gnrc_netif_hdr_t *nif = (gnrc_netif_hdr_t *)tmp->data;
  memset(&shdr.srcmac[0], 8, 0);
  memcpy(&shdr.srcmac[0], gnrc_netif_hdr_get_src_addr(nif), nif->src_l2addr_len);
  shdr.rssi = nif->rssi;
  shdr.lqi = nif->lqi;
  memcpy(&shdr.srcip[0], &ip->src.u8[0], 16);
  shdr.recv_time = xtimer_now_usec64();
  shdr.len = p->size;
  char* payload = (char *)p->data;
  rethos_start_frame(&rethos, (uint8_t*)&shdr, sizeof(staging_hdr_t), CHANNEL_L7G, RETHOS_FRAME_TYPE_DATA);
  rethos_continue_frame(&rethos, (uint8_t*)payload, shdr.len);
  rethos_end_frame(&rethos);
}

void *l7g_main(void *a)
{
    static msg_t _msg_q[Q_SZ];
    msg_t msg, reply;
    reply.type = GNRC_NETAPI_MSG_TYPE_ACK;
    reply.content.value = -ENOTSUP;
    msg_init_queue(_msg_q, Q_SZ);
    gnrc_pktsnip_t *pkt = NULL;
    kernel_pid_t me_pid = thread_getpid();
    gnrc_netreg_entry_t me_reg = GNRC_NETREG_ENTRY_INIT_PID(4747, me_pid);
    gnrc_netreg_register(GNRC_NETTYPE_UDP , &me_reg);
    while (1) {
        msg_receive(&msg);
        switch (msg.type) {
            case GNRC_NETAPI_MSG_TYPE_RCV:
                pkt = msg.content.ptr;
                _handle_incoming_pkt(pkt);
                gnrc_pktbuf_release(pkt);
                break;
             case GNRC_NETAPI_MSG_TYPE_SET:
             case GNRC_NETAPI_MSG_TYPE_GET:
                msg_reply(&msg, &reply);
                break;
            default:
                break;
        }

    }
}
static kernel_pid_t l7g_pid = 0;
static char l7g_stack[1024];
kernel_pid_t start_l7g(void)
{
  if (l7g_pid != 0)
  {
    return l7g_pid;
  }
  l7g_pid = thread_create(l7g_stack, sizeof(l7g_stack),
                          THREAD_PRIORITY_MAIN - 1, THREAD_CREATE_STACKTEST,
                          l7g_main, NULL, "l7g");
  return l7g_pid;
}
