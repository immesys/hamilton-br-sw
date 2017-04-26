
#include <stdio.h>
#include <inttypes.h>

#include "net/gnrc.h"
#include "net/gnrc/ipv6.h"
#include "net/gnrc/udp.h"
#include "net/gnrc/netif/hdr.h"
#include "net/gnrc/pkt.h"
#include <checksum/fletcher16.h>
#include <rethos.h>
#include <board.h>

#define CHANNEL_UPLINK 7

extern ethos_t rethos;

#define Q_SZ 256

extern ipv6_addr_t ipv6_addr;
extern const uint8_t ipv6_prefix_bytes;

static gnrc_pktsnip_t* prep_ipv6_hdr(gnrc_pktsnip_t** pkt, gnrc_pktsnip_t* ipv6_read_only)
{
    gnrc_pktsnip_t* p = *pkt;
    p = gnrc_pktbuf_start_write(p);
    if (p == NULL) {
        return NULL;
    }
    *pkt = p;
    gnrc_pktsnip_t* prev = p;
    p = p->next;

    /* Need write access for everything up to and including the IPv6 header. */
    while (p != ipv6_read_only->next) {
        p = gnrc_pktbuf_start_write(p);
        if (p == NULL) {
            return NULL;
        }
        prev->next = p; // prev already write-protected so this is OK
        prev = p;
        p = p->next;
    }

    gnrc_pktsnip_t* ipv6 = prev;
    gnrc_pktsnip_t* payload = ipv6->next;

    ipv6_hdr_t *hdr = ipv6->data;

    hdr->len = byteorder_htons(gnrc_pkt_len(payload));

    /* check if e.g. extension header was not already marked */
    if (hdr->nh == PROTNUM_RESERVED) {
        hdr->nh = gnrc_nettype_to_protnum(payload->type);

        /* if still reserved: mark no next header */
        if (hdr->nh == PROTNUM_RESERVED) {
            hdr->nh = PROTNUM_IPV6_NONXT;
        }
    }

    if (hdr->hl == 0) {
        hdr->hl = GNRC_IPV6_NETIF_DEFAULT_HL;
    }

    if (ipv6_addr_is_unspecified(&hdr->src)) {
        memcpy(&hdr->src, &ipv6_addr, sizeof(ipv6_addr));
    }

    /* Apparently we may need to calculate the checksum for the upper layer. */
    gnrc_netreg_calc_csum(payload, ipv6);

    return ipv6;
}

gnrc_pktsnip_t* should_send_upstream(gnrc_pktsnip_t* tmp)
{
    gnrc_pktsnip_t* p = NULL;
    LL_SEARCH_SCALAR(tmp, p, type, GNRC_NETTYPE_IPV6);

    if (p == NULL || p->type != GNRC_NETTYPE_IPV6) {
        return NULL;
    }

    /* Packet must be big enough to contain an IP header. */
    if (p->size < sizeof(ipv6_hdr_t)) {
        return NULL;
    }

    /* Check IP address. */
    ipv6_hdr_t* iphdr = p->data;
    if (!ipv6_addr_is_global(&iphdr->dst)) {
        return NULL;
    }
    if (memcmp(&iphdr->dst, &ipv6_addr, ipv6_prefix_bytes) == 0) {
        return NULL;
    }

    return p;
}

void send_upstream(gnrc_pktsnip_t* p) {
    rethos_start_frame(&rethos, NULL, 0, CHANNEL_UPLINK, RETHOS_FRAME_TYPE_DATA);
    while (p != NULL) {
        if (p->type != GNRC_NETTYPE_NETIF) {
            rethos_continue_frame(&rethos, p->data, p->size);
        }
        p = p->next;
    }
    rethos_end_frame(&rethos);
}

void* br_main(void *a)
{
    static msg_t _msg_q[Q_SZ];
    msg_t msg, reply;
    reply.type = GNRC_NETAPI_MSG_TYPE_ACK;
    reply.content.value = -ENOTSUP;
    msg_init_queue(_msg_q, Q_SZ);
    gnrc_pktsnip_t* pkt = NULL;
    gnrc_pktsnip_t* ipv6_read_only = NULL;
    gnrc_pktsnip_t* ipv6 = NULL;
    kernel_pid_t me_pid = thread_getpid();
    gnrc_netreg_entry_t me_reg = GNRC_NETREG_ENTRY_INIT_PID(GNRC_NETREG_DEMUX_CTX_ALL, me_pid);
    gnrc_netreg_register(GNRC_NETTYPE_IPV6 , &me_reg);
    while (1) {
        msg_receive(&msg);
        switch (msg.type) {
            case GNRC_NETAPI_MSG_TYPE_SND:
                pkt = msg.content.ptr;
                ipv6_read_only = should_send_upstream(pkt);
                if (ipv6_read_only == NULL) {
                    gnrc_pktbuf_release(pkt);
                    break;
                }
                ipv6 = prep_ipv6_hdr(&pkt, ipv6_read_only);
                if (ipv6 == NULL) {
                    gnrc_pktbuf_release(pkt);
                    break;
                }
                send_upstream(ipv6);
                gnrc_pktbuf_release(pkt);
                break;
            case GNRC_NETAPI_MSG_TYPE_RCV:
                pkt = msg.content.ptr;
                ipv6_read_only = should_send_upstream(pkt);
                if (ipv6_read_only == NULL) {
                    gnrc_pktbuf_release(pkt);
                    break;
                }
                send_upstream(ipv6_read_only);
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
static kernel_pid_t br_pid = 0;
static char br_stack[1024];
kernel_pid_t start_br(void)
{
  if (br_pid != 0)
  {
    return br_pid;
  }
  br_pid = thread_create(br_stack, sizeof(br_stack),
                          THREAD_PRIORITY_MAIN - 1, THREAD_CREATE_STACKTEST,
                          br_main, NULL, "br");
  return br_pid;
}
