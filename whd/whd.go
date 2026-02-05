// package whd implements Cypress' Wifi Host Driver controller interface API.
// It also contains values common to the cy43 driver.

package whd

// CountryCode returns the country code representation given an uppercase two character string.
// Use rev=0 if unsure.
// Known countries:
//   - WORLDWIDE      "XX" (use this if in doubt)
//   - AUSTRALIA      "AU"
//   - AUSTRIA        "AT"
//   - BELGIUM        "BE"
//   - BRAZIL         "BR"
//   - CANADA         "CA"
//   - CHILE          "CL"
//   - CHINA          "CN"
//   - COLOMBIA       "CO"
//   - CZECH_REPUBLIC "CZ"
//   - DENMARK        "DK"
//   - ESTONIA        "EE"
//   - FINLAND        "FI"
//   - FRANCE         "FR"
//   - GERMANY        "DE"
//   - GREECE         "GR"
//   - HONG_KONG      "HK"
//   - HUNGARY        "HU"
//   - ICELAND        "IS"
//   - INDIA          "IN"
//   - ISRAEL         "IL"
//   - ITALY          "IT"
//   - JAPAN          "JP"
//   - KENYA          "KE"
//   - LATVIA         "LV"
//   - LIECHTENSTEIN  "LI"
//   - LITHUANIA      "LT"
//   - LUXEMBOURG     "LU"
//   - MALAYSIA       "MY"
//   - MALTA          "MT"
//   - MEXICO         "MX"
//   - NETHERLANDS    "NL"
//   - NEW_ZEALAND    "NZ"
//   - NIGERIA        "NG"
//   - NORWAY         "NO"
//   - PERU           "PE"
//   - PHILIPPINES    "PH"
//   - POLAND         "PL"
//   - PORTUGAL       "PT"
//   - SINGAPORE      "SG"
//   - SLOVAKIA       "SK"
//   - SLOVENIA       "SI"
//   - SOUTH_AFRICA   "ZA"
//   - SOUTH_KOREA    "KR"
//   - SPAIN          "ES"
//   - SWEDEN         "SE"
//   - SWITZERLAND    "CH"
//   - TAIWAN         "TW"
//   - THAILAND       "TH"
//   - TURKEY         "TR"
//   - UK             "GB"
//   - USA            "US"
//
//go:inline
func CountryInfo(s string, rev uint8) (info [12]byte) {
	if len(s) != 2 || s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' {
		return // bad country code.
	}
	info[0] = s[0]
	info[1] = s[1]
	if rev == 0 {
		info[4] = 0xff
		info[5] = 0xff
		info[6] = 0xff
		info[7] = 0xff
	} else {
		info[4] = rev
	}
	info[8] = s[0]
	info[9] = s[1]
	return
}

const (
	SDPCM_HEADER_LEN = 12
	IOCTL_HEADER_LEN = 16
	BDC_HEADER_LEN   = 4
	CDC_HEADER_LEN   = 16
	DL_HEADER_LEN    = 12 // DownloadHeader size.
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
	CORE_WLAN_ARM = 1
	CORE_SOCSRAM  = 2

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
	SBSDIO_ALP_AVAIL           = 0x40
	SBSDIO_FORCE_HW_CLKREQ_OFF = 0x20
	SBSDIO_ALP_AVAIL_REQ       = 0x08
	SBSDIO_FORCE_ALP           = 0x01
	SBSDIO_FORCE_HT            = 0x02
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

// IoctlInterfaces.
const (
	IF_STA IoctlInterface = 0
	IF_AP  IoctlInterface = 1
	IF_P2P IoctlInterface = 2
)

func (i IoctlInterface) IsValid() bool {
	return i <= IF_P2P
}

func (i IoctlInterface) String() (s string) {
	switch i {
	case IF_STA:
		s = "sta"
	case IF_AP:
		s = "ap"
	case IF_P2P:
		s = "p2p"
	default:
		s = "unknown"
	}
	return s
}

type SDPCMHeaderType uint8

// for cyw43_sdpcm_send_common
const (
	CONTROL_HEADER    SDPCMHeaderType = 0
	ASYNCEVENT_HEADER SDPCMHeaderType = 1
	DATA_HEADER       SDPCMHeaderType = 2
	UNKNOWN_HEADER    SDPCMHeaderType = 0xff

	CDCF_IOC_ID_SHIFT = 16
	CDCF_IOC_ID_MASK  = 0xffff0000
	CDCF_IOC_IF_SHIFT = 12
)

func (ht SDPCMHeaderType) String() (s string) {
	switch ht {
	case CONTROL_HEADER:
		s = "ctl"
	case ASYNCEVENT_HEADER:
		s = "asyncev"
	case DATA_HEADER:
		s = "data"
	default:
		s = "UNKNOWN"
	}
	return s
}

// kinds of ioctl commands.
const (
	SDPCM_GET = 0
	SDPCM_SET = 2
)

//go:generate stringer -type=SDPCMCommand -output=sdpcm_command_string.go -trimprefix=WLC_

type SDPCMCommand uint32

const (
	WLC_UP            SDPCMCommand = 2
	WLC_DOWN          SDPCMCommand = 3
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
	WLC_SET_AP        SDPCMCommand = 118
	WLC_SET_WSEC      SDPCMCommand = 134
	WLC_SET_BAND      SDPCMCommand = 142
	WLC_GET_ASSOCLIST SDPCMCommand = 159
	WLC_SET_WPA_AUTH  SDPCMCommand = 165
	WLC_SET_VAR       SDPCMCommand = 263
	WLC_GET_VAR       SDPCMCommand = 262
	WLC_SET_WSEC_PMK  SDPCMCommand = 268
)

func (cmd SDPCMCommand) IsValid() bool {
	return cmd == WLC_UP || cmd == WLC_DOWN || cmd == WLC_SET_INFRA || cmd == WLC_SET_AUTH || cmd == WLC_GET_BSSID ||
		cmd == WLC_GET_SSID || cmd == WLC_SET_SSID || cmd == WLC_SET_CHANNEL || cmd == WLC_DISASSOC ||
		cmd == WLC_GET_ANTDIV || cmd == WLC_SET_ANTDIV || cmd == WLC_SET_DTIMPRD || cmd == WLC_GET_PM ||
		cmd == WLC_SET_PM || cmd == WLC_SET_GMODE || cmd == WLC_SET_AP || cmd == WLC_SET_WSEC || cmd == WLC_SET_BAND ||
		cmd == WLC_GET_ASSOCLIST || cmd == WLC_SET_WPA_AUTH || cmd == WLC_SET_VAR || cmd == WLC_GET_VAR ||
		cmd == WLC_SET_WSEC_PMK
}

// SDIO bus specifics
const (
	SDIOD_CCCR_IOEN          = 0x02
	SDIOD_CCCR_IORDY         = 0x03
	SDIOD_CCCR_INTEN         = 0x04
	SDIOD_CCCR_BICTRL        = 0x07
	SDIOD_CCCR_BLKSIZE_0     = 0x10
	SDIOD_CCCR_SPEED_CONTROL = 0x13
	SDIOD_CCCR_BRCM_CARDCAP  = 0xf0
	SDIOD_SEP_INT_CTL        = 0xf2
	SDIOD_CCCR_F1BLKSIZE_0   = 0x110
	SDIOD_CCCR_F2BLKSIZE_0   = 0x210
	SDIOD_CCCR_F2BLKSIZE_1   = 0x211
	INTR_CTL_MASTER_EN       = 0x01
	INTR_CTL_FUNC1_EN        = 0x02
	INTR_CTL_FUNC2_EN        = 0x04
	SDIO_FUNC_ENABLE_1       = 0x02
	SDIO_FUNC_ENABLE_2       = 0x04
	SDIO_FUNC_READY_1        = 0x02
	SDIO_FUNC_READY_2        = 0x04
	SDIO_64B_BLOCK           = 64
	SDIO_PULL_UP             = 0x1000f
)

// SDIOD_CCCR_BRCM_CARDCAP bits
const (
	SDIOD_CCCR_BRCM_CARDCAP_CMD14_SUPPORT = 0x02 // Supports CMD14
	SDIOD_CCCR_BRCM_CARDCAP_CMD14_EXT     = 0x04 // CMD14 is allowed in FSM command state
	SDIOD_CCCR_BRCM_CARDCAP_CMD_NODEC     = 0x08 // sdiod_aos does not decode any command
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

// SPI Definitions.

// Test register value
const TEST_PATTERN = 0xFEEDBEAD

// Register addresses
const (
	SPI_BUS_CONTROL               = 0x0000
	SPI_RESPONSE_DELAY            = 0x0001
	SPI_STATUS_ENABLE             = 0x0002
	SPI_RESET_BP                  = 0x0003 // (corerev >= 1)
	SPI_INTERRUPT_REGISTER        = 0x0004 // 16 bits - Interrupt status
	SPI_INTERRUPT_ENABLE_REGISTER = 0x0006 // 16 bits - Interrupt mask
	SPI_STATUS_REGISTER           = 0x0008 // 32 bits
	SPI_FUNCTION1_INFO            = 0x000C // 16 bits
	SPI_FUNCTION2_INFO            = 0x000E // 16 bits
	SPI_FUNCTION3_INFO            = 0x0010 // 16 bits
	// 32 bit address that contains only-read 0xFEEDBEAD value.
	SPI_READ_TEST_REGISTER = 0x0014 // 32 bits
	SPI_RESP_DELAY_F0      = 0x001c // 8 bits (corerev >= 3)
	SPI_RESP_DELAY_F1      = 0x001d // 8 bits (corerev >= 3)
	SPI_RESP_DELAY_F2      = 0x001e // 8 bits (corerev >= 3)
	SPI_RESP_DELAY_F3      = 0x001f // 8 bits (corerev >= 3)
)

// SPI_FUNCTIONX_BITS
const (
	SPI_FUNCTIONX_ENABLED = 1 << 0
	SPI_FUNCTIONX_READY   = 1 << 1
)

// SPI_BUS_CONTROL Bits
const (
	WORD_LENGTH_32          = 0x01 // 0/1 16/32 bit word length
	ENDIAN_BIG              = 0x02 // 0/1 Little/Big Endian
	CLOCK_PHASE             = 0x04 // 0/1 clock phase delay
	CLOCK_POLARITY          = 0x08 // 0/1 Idle state clock polarity is low/high
	HIGH_SPEED_MODE         = 0x10 // 1/0 High Speed mode / Normal mode
	INTERRUPT_POLARITY_HIGH = 0x20 // 1/0 Interrupt active polarity is high/low
	WAKE_UP                 = 0x80 // 0/1 Wake-up command from Host to WLAN
)

// SPI_STATUS_ENABLE bits
const (
	STATUS_ENABLE    = 0x01 // 1/0 Status sent/not sent to host after read/write
	INTR_WITH_STATUS = 0x02 // 0/1 Do-not / do-interrupt if status is sent
	RESP_DELAY_ALL   = 0x04 // Applicability of resp delay to F1 or all func's read
	DWORD_PKT_LEN_EN = 0x08 // Packet len denoted in dwords instead of bytes
	CMD_ERR_CHK_EN   = 0x20 // Command error check enable
	DATA_ERR_CHK_EN  = 0x40 // Data error check enable
)

// SPI_INTERRUPT_REGISTER and SPI_INTERRUPT_ENABLE_REGISTER bits
const (
	DATA_UNAVAILABLE        = 0x0001 // Requested data not available; Clear by writing a "1"
	F2_F3_FIFO_RD_UNDERFLOW = 0x0002
	F2_F3_FIFO_WR_OVERFLOW  = 0x0004
	COMMAND_ERROR           = 0x0008 // Cleared by writing 1
	DATA_ERROR              = 0x0010 // Cleared by writing 1
	F2_PACKET_AVAILABLE     = 0x0020
	F3_PACKET_AVAILABLE     = 0x0040
	F1_OVERFLOW             = 0x0080 // Due to last write. Bkplane has pending write requests
	GSPI_PACKET_AVAILABLE   = 0x0100
	MISC_INTR1              = 0x0200
	MISC_INTR2              = 0x0400
	MISC_INTR3              = 0x0800
	MISC_INTR4              = 0x1000
	F1_INTR                 = 0x2000
	F2_INTR                 = 0x4000
	F3_INTR                 = 0x8000
)

const BUS_OVERFLOW_UNDERFLOW = F1_OVERFLOW | F2_F3_FIFO_RD_UNDERFLOW | F2_F3_FIFO_WR_OVERFLOW

// SPI_STATUS_REGISTER bits
const (
	STATUS_DATA_NOT_AVAILABLE = 0x00000001
	STATUS_UNDERFLOW          = 0x00000002
	STATUS_OVERFLOW           = 0x00000004
	STATUS_F2_INTR            = 0x00000008
	STATUS_F3_INTR            = 0x00000010
	STATUS_F2_RX_READY        = 0x00000020
	STATUS_F3_RX_READY        = 0x00000040
	STATUS_HOST_CMD_DATA_ERR  = 0x00000080
	STATUS_F2_PKT_AVAILABLE   = 0x00000100
	STATUS_F2_PKT_LEN_MASK    = 0x000FFE00
	STATUS_F2_PKT_LEN_SHIFT   = 9
	STATUS_F3_PKT_AVAILABLE   = 0x00100000
	STATUS_F3_PKT_LEN_MASK    = 0xFFE00000
	STATUS_F3_PKT_LEN_SHIFT   = 21
)

const (
	BUS_SPI_MAX_BACKPLANE_TRANSFER_SIZE = 64 // Max packet size on F1
	BUS_SPI_BACKPLANE_READ_PADD_SIZE    = 4

	SPI_FRAME_CONTROL = 0x1000D
)

// Async events, event_type field
const (
	CYW43_EV_SET_SSID                 = 0
	CYW43_EV_JOIN                     = 1
	CYW43_EV_AUTH                     = 3
	CYW43_EV_DEAUTH                   = 5
	CYW43_EV_DEAUTH_IND               = 6
	CYW43_EV_ASSOC                    = 7
	CYW43_EV_DISASSOC                 = 11
	CYW43_EV_DISASSOC_IND             = 12
	CYW43_EV_LINK                     = 16
	CYW43_EV_PRUNE                    = 23
	CYW43_EV_PSK_SUP                  = 46
	CYW43_EV_IF                       = 54 // I/F change (for wlan host notification)
	CYW43_EV_P2P_DISC_LISTEN_COMPLETE = 55 // P2P Discovery listen state expires
	CYW43_EV_RSSI                     = 56 // indicate RSSI change based on configured levels
	CYW43_EV_ESCAN_RESULT             = 69
	CYW43_EV_CSA_COMPLETE_IND         = 80
	CYW43_EV_ASSOC_REQ_IE             = 87
	CYW43_EV_ASSOC_RESP_IE            = 88
)

// IOCTL commands
const (
	CYW43_IOCTL_GET_SSID     = 0x32
	CYW43_IOCTL_GET_CHANNEL  = 0x3a
	CYW43_IOCTL_SET_DISASSOC = 0x69
	CYW43_IOCTL_GET_ANTDIV   = 0x7e
	CYW43_IOCTL_SET_ANTDIV   = 0x81
	CYW43_IOCTL_SET_MONITOR  = 0xd9
	CYW43_IOCTL_GET_VAR      = 0x20c
	CYW43_IOCTL_SET_VAR      = 0x20f
)

// Event status values
const (
	CYW43_STATUS_SUCCESS     = 0
	CYW43_STATUS_FAIL        = 1
	CYW43_STATUS_TIMEOUT     = 2
	CYW43_STATUS_NO_NETWORKS = 3
	CYW43_STATUS_ABORT       = 4
	CYW43_STATUS_NO_ACK      = 5
	CYW43_STATUS_UNSOLICITED = 6
	CYW43_STATUS_ATTEMPT     = 7
	CYW43_STATUS_PARTIAL     = 8
	CYW43_STATUS_NEWSCAN     = 9
	CYW43_STATUS_NEWASSOC    = 10
)

// Values used for STA and AP auth settings
const (
	CYW43_SUP_DISCONNECTED       = 0                          // Disconnected
	CYW43_SUP_CONNECTING         = 1                          // Connecting
	CYW43_SUP_IDREQUIRED         = 2                          // ID Required
	CYW43_SUP_AUTHENTICATING     = 3                          // Authenticating
	CYW43_SUP_AUTHENTICATED      = 4                          // Authenticated
	CYW43_SUP_KEYXCHANGE         = 5                          // Key Exchange
	CYW43_SUP_KEYED              = 6                          // Key Exchanged
	CYW43_SUP_TIMEOUT            = 7                          // Timeout
	CYW43_SUP_LAST_BASIC_STATE   = 8                          // Last Basic State
	CYW43_SUP_KEYXCHANGE_WAIT_M1 = CYW43_SUP_AUTHENTICATED    // Waiting to receive handshake msg M1
	CYW43_SUP_KEYXCHANGE_PREP_M2 = CYW43_SUP_KEYXCHANGE       // Preparing to send handshake msg M2
	CYW43_SUP_KEYXCHANGE_WAIT_M3 = CYW43_SUP_LAST_BASIC_STATE // Waiting to receive handshake msg M3
	CYW43_SUP_KEYXCHANGE_PREP_M4 = 9                          // Preparing to send handshake msg M4
	CYW43_SUP_KEYXCHANGE_WAIT_G1 = 10                         // Waiting to receive handshake msg G1
	CYW43_SUP_KEYXCHANGE_PREP_G2 = 11                         // Preparing to send handshake msg G2
)

// Values for AP auth setting
const (
	CYW43_REASON_INITIAL_ASSOC    = 0 // initial assoc
	CYW43_REASON_LOW_RSSI         = 1 // roamed due to low RSSI
	CYW43_REASON_DEAUTH           = 2 // roamed due to DEAUTH indication
	CYW43_REASON_DISASSOC         = 3 // roamed due to DISASSOC indication
	CYW43_REASON_BCNS_LOST        = 4 // roamed due to lost beacons
	CYW43_REASON_FAST_ROAM_FAILED = 5 // roamed due to fast roam failure
	CYW43_REASON_DIRECTED_ROAM    = 6 // roamed due to request by AP
	CYW43_REASON_TSPEC_REJECTED   = 7 // roamed due to TSPEC rejection
	CYW43_REASON_BETTER_AP        = 8 // roamed due to finding better AP
)

// prune reason codes
const (
	CYW43_REASON_PRUNE_ENCR_MISMATCH   = 1  // encryption mismatch
	CYW43_REASON_PRUNE_BCAST_BSSID     = 2  // AP uses a broadcast BSSID
	CYW43_REASON_PRUNE_MAC_DENY        = 3  // STA's MAC addr is in AP's MAC deny list
	CYW43_REASON_PRUNE_MAC_NA          = 4  // STA's MAC addr is not in AP's MAC allow list
	CYW43_REASON_PRUNE_REG_PASSV       = 5  // AP not allowed due to regulatory restriction
	CYW43_REASON_PRUNE_SPCT_MGMT       = 6  // AP does not support STA locale spectrum mgmt
	CYW43_REASON_PRUNE_RADAR           = 7  // AP is on a radar channel of STA locale
	CYW43_REASON_RSN_MISMATCH          = 8  // STA does not support AP's RSN
	CYW43_REASON_PRUNE_NO_COMMON_RATES = 9  // No rates in common with AP
	CYW43_REASON_PRUNE_BASIC_RATES     = 10 // STA does not support all basic rates of BSS
	CYW43_REASON_PRUNE_CCXFAST_PREVAP  = 11 // CCX FAST ROAM: prune previous AP
	CYW43_REASON_PRUNE_CIPHER_NA       = 12 // BSS's cipher not supported
	CYW43_REASON_PRUNE_KNOWN_STA       = 13 // AP is already known to us as a STA
	CYW43_REASON_PRUNE_CCXFAST_DROAM   = 14 // CCX FAST ROAM: prune unqualified AP
	CYW43_REASON_PRUNE_WDS_PEER        = 15 // AP is already known to us as a WDS peer
	CYW43_REASON_PRUNE_QBSS_LOAD       = 16 // QBSS LOAD - AAC is too low
	CYW43_REASON_PRUNE_HOME_AP         = 17 // prune home AP
	CYW43_REASON_PRUNE_AP_BLOCKED      = 18 // prune blocked AP
	CYW43_REASON_PRUNE_NO_DIAG_SUPPORT = 19 // prune due to diagnostic mode not supported
)

// WPA failure reason codes carried in the WLC_E_PSK_SUP event
const (
	CYW43_REASON_SUP_OTHER            = 0  // Other reason
	CYW43_REASON_SUP_DECRYPT_KEY_DATA = 1  // Decryption of key data failed
	CYW43_REASON_SUP_BAD_UCAST_WEP128 = 2  // Illegal use of ucast WEP128
	CYW43_REASON_SUP_BAD_UCAST_WEP40  = 3  // Illegal use of ucast WEP40
	CYW43_REASON_SUP_UNSUP_KEY_LEN    = 4  // Unsupported key length
	CYW43_REASON_SUP_PW_KEY_CIPHER    = 5  // Unicast cipher mismatch in pairwise key
	CYW43_REASON_SUP_MSG3_TOO_MANY_IE = 6  // WPA IE contains > 1 RSN IE in key msg 3
	CYW43_REASON_SUP_MSG3_IE_MISMATCH = 7  // WPA IE mismatch in key message 3
	CYW43_REASON_SUP_NO_INSTALL_FLAG  = 8  // INSTALL flag unset in 4-way msg
	CYW43_REASON_SUP_MSG3_NO_GTK      = 9  // encapsulated GTK missing from msg 3
	CYW43_REASON_SUP_GRP_KEY_CIPHER   = 10 // Multicast cipher mismatch in group key
	CYW43_REASON_SUP_GRP_MSG1_NO_GTK  = 11 // encapsulated GTK missing from group msg 1
	CYW43_REASON_SUP_GTK_DECRYPT_FAIL = 12 // GTK decrypt failure
	CYW43_REASON_SUP_SEND_FAIL        = 13 // message send failure
	CYW43_REASON_SUP_DEAUTH           = 14 // received FC_DEAUTH
	CYW43_REASON_SUP_WPA_PSK_TMO      = 15 // WPA PSK 4-way handshake timeout
)

// Values used for STA and AP auth settings
const (
	CYW43_WPA_AUTH_PSK  = 0x0004
	CYW43_WPA2_AUTH_PSK = 0x0080
)

// The following constants are from embassy-rs cyw43 driver:
// https://github.com/embassy-rs/embassy/blob/main/cyw43/src/consts.rs

// Wireless Security (WSEC) cipher flags for WLC_SET_WSEC ioctl.
const (
	WSEC_NONE uint32 = 0x00
	WSEC_WEP  uint32 = 0x01
	WSEC_TKIP uint32 = 0x02
	WSEC_AES  uint32 = 0x04
)

// Management Frame Protection (MFP) modes for "mfp" iovar.
// Required for proper WPA2/WPA3 cipher negotiation.
const (
	MFP_NONE     uint32 = 0 // No MFP
	MFP_CAPABLE  uint32 = 1 // MFP capable (recommended for WPA2)
	MFP_REQUIRED uint32 = 2 // MFP required (required for WPA3)
)

// WPA Authentication modes for WLC_SET_WPA_AUTH ioctl.
// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/consts.rs#L732-L735
const (
	WPA_AUTH_DISABLED    uint32 = 0x0000
	WPA_AUTH_WPA_PSK     uint32 = 0x0004
	WPA_AUTH_WPA2_PSK    uint32 = 0x0080
	WPA_AUTH_WPA3_SAE_PSK uint32 = 0x40000
)

// Open system authentication for WLC_SET_AUTH ioctl.
const (
	AUTH_OPEN uint32 = 0x00
	AUTH_SAE  uint32 = 0x03 // SAE (Simultaneous Authentication of Equals) for WPA3
)

// # Authorization types
//
// Used when setting up an access point, or connecting to an access point
const (
	CYW43_AUTH_OPEN           = 0          ///< No authorisation required (open)
	CYW43_AUTH_WPA_TKIP_PSK   = 0x00200002 ///< WPA authorisation
	CYW43_AUTH_WPA2_AES_PSK   = 0x00400004 ///< WPA2 authorisation (preferred)
	CYW43_AUTH_WPA2_MIXED_PSK = 0x00400006 ///< WPA2/WPA mixed authorisation
)

// Passphrase min/max lengths
const (
	CYW43_MIN_PSK_LEN = 8
	CYW43_MAX_PSK_LEN = 64
)

// Power save mode paramter passed to cyw43_ll_wifi_pm
const (
	CYW43_NO_POWERSAVE_MODE  = 0 ///< No Powersave mode
	CYW43_PM1_POWERSAVE_MODE = 1 ///< Powersave mode on specified interface without regard for throughput reduction
	CYW43_PM2_POWERSAVE_MODE = 2 ///< Powersave mode on specified interface with High throughput
)

// Link status
const (
	CYW43_LINK_DOWN    = 0  ///< link is down
	CYW43_LINK_JOIN    = 1  ///< Connected to wifi
	CYW43_LINK_NOIP    = 2  ///< Connected to wifi, but no IP address
	CYW43_LINK_UP      = 3  ///< Connect to wifi with an IP address
	CYW43_LINK_FAIL    = -1 ///< Connection failed
	CYW43_LINK_NONET   = -2 ///< No matching SSID found (could be out of range, or down)
	CYW43_LINK_BADAUTH = -3 ///< Authenticatation failure
)

// To indicate no specific channel when calling cyw43_ll_wifi_join with bssid specified
const CYW43_CHANNEL_NONE = 0xffffffff ///< No Channel specified (use the AP's channel)

// Network interface types
const (
	CYW43_ITF_STA = 0
	CYW43_ITF_AP  = 1
)

// Bits 0-3 are an enumeration, while bits 8-11 are flags.
const (
	WIFI_JOIN_STATE_KIND_MASK = 0x000f
	WIFI_JOIN_STATE_DOWN      = 0x0000
	WIFI_JOIN_STATE_ACTIVE    = 0x0001
	WIFI_JOIN_STATE_FAIL      = 0x0002
	WIFI_JOIN_STATE_NONET     = 0x0003
	WIFI_JOIN_STATE_BADAUTH   = 0x0004
	WIFI_JOIN_STATE_AUTH      = 0x0200
	WIFI_JOIN_STATE_LINK      = 0x0400
	WIFI_JOIN_STATE_KEYED     = 0x0800
	WIFI_JOIN_STATE_ALL       = 0x0e01
)

// Bluetooth constants.
const (
	BT2WLAN_PWRUP_WAKE = 3
	BT2WLAN_PWRUP_ADDR = 0x640894

	CYW_BT_BASE_ADDRESS    = 0x19000000
	BT_CTRL_REG_ADDR       = 0x18000c7c
	HOST_CTRL_REG_ADDR     = 0x18000d6c
	WLAN_RAM_BASE_REG_ADDR = 0x18000d68

	BTSDIO_REG_DATA_VALID_BITMASK = 1 << 1
	BTSDIO_REG_BT_AWAKE_BITMASK   = 1 << 8
	BTSDIO_REG_WAKE_BT_BITMASK    = 1 << 17
	BTSDIO_REG_SW_RDY_BITMASK     = 1 << 24
	BTSDIO_REG_FW_RDY_BITMASK     = 1 << 24
	BTSDIO_FWBUF_SIZE             = 0x1000
	BTSDIO_OFFSET_HOST_WRITE_BUF  = 0
	BTSDIO_OFFSET_HOST_READ_BUF   = BTSDIO_FWBUF_SIZE
	BTSDIO_OFFSET_HOST2BT_IN      = 0x00002000
	BTSDIO_OFFSET_HOST2BT_OUT     = 0x00002004
	BTSDIO_OFFSET_BT2HOST_IN      = 0x00002008
	BTSDIO_OFFSET_BT2HOST_OUT     = 0x0000200C

	REG_BACKPLANE_FUNCTION2_WATERMARK = 0x10008
)

// Bluetooth firmware extraction constants.
const (
	BTFW_ADDR_MODE_UNKNOWN  = 0
	BTFW_ADDR_MODE_EXTENDED = 1
	BTFW_ADDR_MODE_SEGMENT  = 2
	BTFW_ADDR_MODE_LINEAR32 = 3

	BTFW_HEX_LINE_TYPE_DATA                     = 0
	BTFW_HEX_LINE_TYPE_END_OF_DATA              = 1
	BTFW_HEX_LINE_TYPE_EXTENDED_SEGMENT_ADDRESS = 2
	BTFW_HEX_LINE_TYPE_EXTENDED_ADDRESS         = 4
	BTFW_HEX_LINE_TYPE_ABSOLUTE_32BIT_ADDRESS   = 5
)
