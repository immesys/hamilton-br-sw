# name of your application
APPLICATION = rethos-br

# If no BOARD is found in the environment, use this default:
BOARD = samr21-xpro

# This has to be the absolute path to the RIOT base directory:
RIOTBASE = $(CURDIR)/../RIOT

BOARD_INSUFFICIENT_MEMORY := airfy-beacon cc2650stk msb-430 msb-430h pca10000 pca10005 \
                             nrf51dongle nrf6310 nucleo-f103 nucleo-f334 \
                             spark-core stm32f0discovery telosb \
                             weio wsn430-v1_3b wsn430-v1_4 yunjia-nrf51822 z1 nucleo-f072

# use ethos (ethernet over serial) for network communication and stdio over
# UART, but not on native, as native has a tap interface towards the host.
ifeq (,$(filter native,$(BOARD)))
GNRC_NETIF_NUMOF := 2
USEMODULE += rethos
USEMODULE += random
USEMODULE += gnrc_netdev2
CFLAGS += -DRTT_STDIO_DISABLE_STDIN
USEMODULE += rtt_stdio

USEMODULE += at86rf233

# Prefix length is assumed to be 64
CFLAGS += -DHAMILTON_BORDER_ROUTER_ADDRESS=\"fc00::1\"

# Unset this for the Border Router... though you also would need to use the
# Border Router kernel (which is needed to work properly with the samr21 and
# rethos).
CFLAGS += -DI_AM_HAMILTON_BORDER_ROUTER

# rethos baudrate can be configured from make command
RETHOS_BAUDRATE ?= 921600
CFLAGS += '-DRETHOS_UART=UART_DEV(0)' -DRETHOS_BAUDRATE=$(RETHOS_BAUDRATE)
FEATURES_REQUIRED += periph_uart
endif

# Include packages that pull up and auto-init the link layer.
# NOTE: 6LoWPAN will be included if IEEE802.15.4 devices are present
USEMODULE += gnrc_ipv6_autoconf_onehop
USEMODULE += gnrc_netdev_default
USEMODULE += gnrc_dutymac
USEMODULE += gnrc_udp
USEMODULE += netdev2_eth
USEMODULE += auto_init_gnrc_netif
USEMODULE += gnrc_sixlowpan_frag

# Add forwarding table
USEMODULE += fib
# Additional networking modules that can be dropped if not needed
USEMODULE += gnrc_icmpv6_echo
# Add also the shell, some shell commands
# USEMODULE += shell
# USEMODULE += shell_commands
# USEMODULE += ps


# Comment this out to disable code in RIOT that does safety checking
# which is not needed in a production environment but helps in the
# development process:
# CFLAGS += -DDEVELHELP

# Set a custom 802.15.4 channel if needed
DEFAULT_CHANNEL = 25
CFLAGS += -DIEEE802154_DEFAULT_CHANNEL=$(DEFAULT_CHANNEL)

# Comment this out to disable code in RIOT that does safety checking
# which is not needed in a production environment but helps in the
# development process:
# CFLAGS += -DDEVELHELP

# Change this to 0 show compiler invocation lines by default:
QUIET ?= 1

include $(RIOTBASE)/Makefile.include
