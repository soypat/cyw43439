// package whd implements Cypress' Wifi Host Driver controller interface API.
package whd

const (
	SDPCM_HEADER_LEN = 12
	IOCTL_HEADER_LEN = 16
)

const (
	SDIO_FUNCTION2_WATERMARK    = 0x10008
	SDIO_BACKPLANE_ADDRESS_LOW  = 0x1000a
	SDIO_BACKPLANE_ADDRESS_MID  = 0x1000b
	SDIO_BACKPLANE_ADDRESS_HIGH = 0x1000c
	SDIO_CHIP_CLOCK_CSR         = 0x1000e
	SDIO_WAKEUP_CTRL            = 0x1001e
	SDIO_SLEEP_CSR              = 0x1001f
)

const (
	I_HMB_SW_MASK   = 0x000000f0
	I_HMB_FC_CHANGE = 1 << 5
)

const (
	CHIPCOMMON_BASE_ADDRESS  = 0x18000000
	SDIO_BASE_ADDRESS        = 0x18002000
	WLAN_ARMCM3_BASE_ADDRESS = 0x18003000
	SOCSRAM_BASE_ADDRESS     = 0x18004000
	BACKPLANE_ADDR_MASK      = 0x7fff
	WRAPPER_REGISTER_OFFSET  = 0x100000

	SBSDIO_SB_ACCESS_2_4B_FLAG = 0x08000

	CHIPCOMMON_SR_CONTROL1 = CHIPCOMMON_BASE_ADDRESS + 0x508
	SDIO_INT_STATUS        = SDIO_BASE_ADDRESS + 0x20
	SDIO_INT_HOST_MASK     = SDIO_BASE_ADDRESS + 0x24
	SDIO_FUNCTION_INT_MASK = SDIO_BASE_ADDRESS + 0x34
	SDIO_TO_SB_MAILBOX     = SDIO_BASE_ADDRESS + 0x40
	SOCSRAM_BANKX_INDEX    = SOCSRAM_BASE_ADDRESS + 0x10
	SOCSRAM_BANKX_PDA      = SOCSRAM_BASE_ADDRESS + 0x44
)

// SDIO_CHIP_CLOCK_CSR bits
const (
	SBSDIO_ALP_AVAIL           = (0x40)
	SBSDIO_FORCE_HW_CLKREQ_OFF = (0x20)
	SBSDIO_ALP_AVAIL_REQ       = (0x08)
	SBSDIO_FORCE_ALP           = (0x01)
	SBSDIO_FORCE_HT            = (0x02)
)

const (
	SBSDIO_HT_AVAIL_REQ = 0x10
	SBSDIO_HT_AVAIL     = 0x80
)

const (
	AI_IOCTRL_OFFSET    = 0x408
	SICF_CPUHALT        = 0x0020
	SICF_FGC            = 0x0002
	SICF_CLOCK_EN       = 0x0001
	AI_RESETCTRL_OFFSET = 0x800
	AIRC_RESET          = 1

	SPI_F2_WATERMARK  = 32
	SDIO_F2_WATERMARK = 8
)

type IoctlInterface uint8

const (
	WWD_STA_INTERFACE IoctlInterface = 0
	WWD_AP_INTERFACE  IoctlInterface = 1
	WWD_P2P_INTERFACE IoctlInterface = 2
)

// for cyw43_sdpcm_send_common
const (
	CONTROL_HEADER    = 0
	ASYNCEVENT_HEADER = 1
	DATA_HEADER       = 2
	CDCF_IOC_ID_SHIFT = (16)
	CDCF_IOC_ID_MASK  = (0xffff0000)
	CDCF_IOC_IF_SHIFT = (12)
)

const (
	SDPCM_GET = 0
	SDPCM_SET = 2
)

type SDPCMCommand uint32

const (
	WLC_UP            SDPCMCommand = 2
	WLC_SET_INFRA     SDPCMCommand = 20
	WLC_SET_AUTH      SDPCMCommand = 22
	WLC_GET_BSSID     SDPCMCommand = 23
	WLC_GET_SSID      SDPCMCommand = 25
	WLC_SET_SSID      SDPCMCommand = 26
	WLC_SET_CHANNEL   SDPCMCommand = 30
	WLC_DISASSOC      SDPCMCommand = 52
	WLC_GET_ANTDIV    SDPCMCommand = 63
	WLC_SET_ANTDIV    SDPCMCommand = 64
	WLC_SET_DTIMPRD   SDPCMCommand = 78
	WLC_GET_PM        SDPCMCommand = 85
	WLC_SET_PM        SDPCMCommand = 86
	WLC_SET_GMODE     SDPCMCommand = 110
	WLC_SET_WSEC      SDPCMCommand = 134
	WLC_SET_BAND      SDPCMCommand = 142
	WLC_GET_ASSOCLIST SDPCMCommand = 159
	WLC_SET_WPA_AUTH  SDPCMCommand = 165
	WLC_SET_VAR       SDPCMCommand = 263
	WLC_GET_VAR       SDPCMCommand = 262
	WLC_SET_WSEC_PMK  SDPCMCommand = 268
)

// SDIO bus specifics
const (
	SDIOD_CCCR_IOEN          = (0x02)
	SDIOD_CCCR_IORDY         = (0x03)
	SDIOD_CCCR_INTEN         = (0x04)
	SDIOD_CCCR_BICTRL        = (0x07)
	SDIOD_CCCR_BLKSIZE_0     = (0x10)
	SDIOD_CCCR_SPEED_CONTROL = (0x13)
	SDIOD_CCCR_BRCM_CARDCAP  = (0xf0)
	SDIOD_SEP_INT_CTL        = (0xf2)
	SDIOD_CCCR_F1BLKSIZE_0   = (0x110)
	SDIOD_CCCR_F2BLKSIZE_0   = (0x210)
	SDIOD_CCCR_F2BLKSIZE_1   = (0x211)
	INTR_CTL_MASTER_EN       = (0x01)
	INTR_CTL_FUNC1_EN        = (0x02)
	INTR_CTL_FUNC2_EN        = (0x04)
	SDIO_FUNC_ENABLE_1       = (0x02)
	SDIO_FUNC_ENABLE_2       = (0x04)
	SDIO_FUNC_READY_1        = (0x02)
	SDIO_FUNC_READY_2        = (0x04)
	SDIO_64B_BLOCK           = (64)
	SDIO_PULL_UP             = (0x1000f)
)

// SDIOD_CCCR_BRCM_CARDCAP bits
const (
	SDIOD_CCCR_BRCM_CARDCAP_CMD14_SUPPORT = (0x02) // Supports CMD14
	SDIOD_CCCR_BRCM_CARDCAP_CMD14_EXT     = (0x04) // CMD14 is allowed in FSM command state
	SDIOD_CCCR_BRCM_CARDCAP_CMD_NODEC     = (0x08) // sdiod_aos does not decode any command
)

// SDIOD_SEP_INT_CTL bits
const (
	SEP_INTR_CTL_MASK = 0x01 // out-of-band interrupt mask
	SEP_INTR_CTL_EN   = 0x02 // out-of-band interrupt output enable
	SEP_INTR_CTL_POL  = 0x04 // out-of-band interrupt polarity

)

// SDIO_WAKEUP_CTRL bits
const (
	SBSDIO_WCTRL_WAKE_TILL_ALP_AVAIL = 1 << 0 // WakeTillAlpAvail bit
	SBSDIO_WCTRL_WAKE_TILL_HT_AVAIL  = 1 << 1 // WakeTillHTAvail bit
)

// SDIO_SLEEP_CSR bits
const (
	SBSDIO_SLPCSR_KEEP_SDIO_ON = 1 << 0 // KeepSdioOn bit
	SBSDIO_SLPCSR_DEVICE_ON    = 1 << 1 // DeviceOn bit
)

// For determining security type from a scan
const (
	DOT11_CAP_PRIVACY           = 0x0010
	DOT11_IE_ID_RSN             = 48
	DOT11_IE_ID_VENDOR_SPECIFIC = 221
	WPA_OUI_TYPE1               = "\x00\x50\xF2\x01"
)

// const SLEEP_MAX (50)

// Multicast registered group addresses
const MAX_MULTICAST_REGISTERED_ADDRESS = 10

// #define CYW_EAPOL_KEY_TIMEOUT (5000)
