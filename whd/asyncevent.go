package whd

// AsyncEventType is the type of an async event
type AsyncEventType uint32

//go:generate stringer -type=AsyncEventType -output=asyncevent_type_string.go -trimprefix=Ev

// Async event types as defined by WHD.
const (
	evUnknown AsyncEventType = 0xFF
	// indicates status of set SSID.
	EvSET_SSID AsyncEventType = 0
	// differentiates join IBSS from found (START) IBSS
	EvJOIN AsyncEventType = 1
	// STA founded an IBSS or AP started a BSS.
	EvSTART AsyncEventType = 2
	// 802.11 AUTH request.
	EvAUTH AsyncEventType = 3
	// 802.11 AUTH indication.
	EvAUTH_IND AsyncEventType = 4
	// 802.11 DEAUTH request.
	EvDEAUTH AsyncEventType = 5
	// 802.11 DEAUTH indication.
	EvDEAUTH_IND AsyncEventType = 6
	// 802.11 ASSOC request.
	EvASSOC AsyncEventType = 7
	// 802.11 ASSOC indication.
	EvASSOC_IND AsyncEventType = 8
	// 802.11 REASSOC request.
	EvREASSOC AsyncEventType = 9
	// 802.11 REASSOC indication.
	EvREASSOC_IND AsyncEventType = 10
	// 802.11 DISASSOC request.
	EvDISASSOC AsyncEventType = 11
	// 802.11 DISASSOC indication.
	EvDISASSOC_IND AsyncEventType = 12
	// 802.11h Quiet period started.
	EvQUIET_START AsyncEventType = 13
	// 802.11h Quiet period ended.
	EvQUIET_END AsyncEventType = 14
	// BEACONS received/lost indication.
	EvBEACON_RX AsyncEventType = 15
	// generic link indication.
	EvLINK AsyncEventType = 16
	// TKIP MIC error occurred.
	EvMIC_ERROR AsyncEventType = 17
	// NDIS style link indication.
	EvNDIS_LINK AsyncEventType = 18
	// roam attempt occurred: indicate status & reason.
	EvROAM AsyncEventType = 19
	// change in dot11FailedCount (txfail
	EvTXFAIL AsyncEventType = 20
	// WPA2 pmkid cache indicatio
	EvPMKID_CACHE AsyncEventType = 21
	// current AP's TSF value went backward
	EvRETROGRADE_TSF AsyncEventType = 22
	// AP was pruned from join list for reason.
	EvPRUNE AsyncEventType = 23
	// report AutoAuth table entry match for join attempt.
	EvAUTOAUTH AsyncEventType = 24
	// Event encapsulating an EAPOL message.
	EvEAPOL_MSG AsyncEventType = 25
	// Scan results are ready or scan was aborted.
	EvSCAN_COMPLETE AsyncEventType = 26
	// indicate to host addts fail/success.
	EvADDTS_IND AsyncEventType = 27
	// indicate to host delts fail/success.
	EvDELTS_IND AsyncEventType = 28
	// indicate to host of beacon transmit.
	EvBCNSENT_IND AsyncEventType = 29
	// Send the received beacon up to the hos
	EvBCNRX_MSG AsyncEventType = 30
	// indicate to host loss of beaco
	EvBCNLOST_MSG AsyncEventType = 31
	// before attempting to roam.
	EvROAM_PREP AsyncEventType = 32
	// PFN network found event.
	EvPFN_NET_FOUND AsyncEventType = 33
	// PFN network lost event.
	EvPFN_NET_LOST   AsyncEventType = 34
	EvRESET_COMPLETE AsyncEventType = 35
	EvJOIN_START     AsyncEventType = 36
	EvROAM_START     AsyncEventType = 37
	EvASSOC_START    AsyncEventType = 38
	EvIBSS_ASSOC     AsyncEventType = 39
	EvRADIO          AsyncEventType = 40
	// PSM microcode watchdog fire
	EvPSM_WATCHDOG AsyncEventType = 41
	// CCX association start.
	EvCCX_ASSOC_START AsyncEventType = 42
	// CCX association abort.
	EvCCX_ASSOC_ABORT AsyncEventType = 43
	// probe request received.
	EvPROBREQ_MSG      AsyncEventType = 44
	EvSCAN_CONFIRM_IND AsyncEventType = 45
	// WPA Handshak
	EvPSK_SUP              AsyncEventType = 46
	EvCOUNTRY_CODE_CHANGED AsyncEventType = 47
	// WMMAC excedded medium tim
	EvEXCEEDED_MEDIUM_TIME AsyncEventType = 48
	// WEP ICV error occurre
	EvICV_ERROR AsyncEventType = 49
	// Unsupported unicast encrypted fram
	EvUNICAST_DECODE_ERROR AsyncEventType = 50
	// Unsupported multicast encrypted fram
	EvMULTICAST_DECODE_ERROR AsyncEventType = 51
	EvTRACE                  AsyncEventType = 52
	// BT-AMP HCI event.
	EvBTA_HCI_EVENT AsyncEventType = 53
	// I/F change (for wlan host notification
	EvIF AsyncEventType = 54
	// P2P Discovery listen state expire
	EvP2P_DISC_LISTEN_COMPLETE AsyncEventType = 55
	// indicate RSSI change based on configured level
	EvRSSI AsyncEventType = 56
	// PFN best network batching event.
	EvPFN_BEST_BATCHING AsyncEventType = 57
	EvEXTLOG_MSG        AsyncEventType = 58
	// Action frame receptio
	EvACTION_FRAME AsyncEventType = 59
	// Action frame Tx complet
	EvACTION_FRAME_COMPLETE AsyncEventType = 60
	// assoc request received.
	EvPRE_ASSOC_IND AsyncEventType = 61
	// re-assoc request received.
	EvPRE_REASSOC_IND AsyncEventType = 62
	// channel adopted (xxx: obsoleted
	EvCHANNEL_ADOPTED AsyncEventType = 63
	// AP starte
	EvAP_STARTED AsyncEventType = 64
	// AP stopped due to DF
	EvDFS_AP_STOP AsyncEventType = 65
	// AP resumed due to DF
	EvDFS_AP_RESUME AsyncEventType = 66
	// WAI stations event.
	EvWAI_STA_EVENT AsyncEventType = 67
	// event encapsulating an WAI messag
	EvWAI_MSG AsyncEventType = 68
	// escan result event.
	EvESCAN_RESULT AsyncEventType = 69
	// action frame off channel complet
	EvACTION_FRAME_OFF_CHAN_COMPLETE AsyncEventType = 70
	// probe response received.
	EvPROBRESP_MSG AsyncEventType = 71
	// P2P Probe request received.
	EvP2P_PROBREQ_MSG AsyncEventType = 72
	EvDCS_REQUEST     AsyncEventType = 73
	// credits for D11 FIFOs. [AC0,AC1,AC2,AC3,BC_MC,ATIM
	EvFIFO_CREDIT_MAP AsyncEventType = 74
	// Received action frame event WITH wl_event_rx_frame_data_t heade
	EvACTION_FRAME_RX AsyncEventType = 75
	// Wake Event timer fired, used for wake WLAN test mod
	EvWAKE_EVENT AsyncEventType = 76
	// Radio measurement complet
	EvRM_COMPLETE AsyncEventType = 77
	// Synchronize TSF with the hos
	EvHTSFSYNC AsyncEventType = 78
	// request an overlay IOCTL/iovar from the hos
	EvOVERLAY_REQ      AsyncEventType = 79
	EvCSA_COMPLETE_IND AsyncEventType = 80
	// excess PM Wake Event to inform hos
	EvEXCESS_PM_WAKE_EVENT AsyncEventType = 81
	// no PFN networks aroun
	EvPFN_SCAN_NONE AsyncEventType = 82
	// last found PFN network gets los
	EvPFN_SCAN_ALLGONE AsyncEventType = 83
	EvGTK_PLUMBED      AsyncEventType = 84
	// 802.11 ASSOC indication for NDIS onl
	EvASSOC_IND_NDIS AsyncEventType = 85
	// 802.11 REASSOC indication for NDIS onl
	EvREASSOC_IND_NDIS AsyncEventType = 86
	EvASSOC_REQ_IE     AsyncEventType = 87
	EvASSOC_RESP_IE    AsyncEventType = 88
	// association recreated on resum
	EvASSOC_RECREATED AsyncEventType = 89
	// rx action frame event for NDIS onl
	EvACTION_FRAME_RX_NDIS AsyncEventType = 90
	// authentication request received.
	EvAUTH_REQ AsyncEventType = 91
	// fast assoc recreation faile
	EvSPEEDY_RECREATE_FAIL AsyncEventType = 93
	// port-specific event and payload (e.g. NDIS
	EvNATIVE AsyncEventType = 94
	// event for tx pkt delay suddently jum
	EvPKTDELAY_IND AsyncEventType = 95
	// AWDL AW period start
	EvAWDL_AW AsyncEventType = 96
	// AWDL Master/Slave/NE master role event.
	EvAWDL_ROLE AsyncEventType = 97
	// Generic AWDL event.
	EvAWDL_EVENT AsyncEventType = 98
	// NIC AF txstatus.
	EvNIC_AF_TXS AsyncEventType = 99
	// NAN event.
	EvNAN             AsyncEventType = 100
	EvBEACON_FRAME_RX AsyncEventType = 101
	// desired service foun
	EvSERVICE_FOUND AsyncEventType = 102
	// GAS fragment received.
	EvGAS_FRAGMENT_RX AsyncEventType = 103
	// GAS sessions all complet
	EvGAS_COMPLETE AsyncEventType = 104
	// New device found by p2p offloa
	EvP2PO_ADD_DEVICE AsyncEventType = 105
	// device has been removed by p2p offloa
	EvP2PO_DEL_DEVICE AsyncEventType = 106
	// WNM event to notify STA enter sleep mod
	EvWNM_STA_SLEEP AsyncEventType = 107
	// Indication of MAC tx failures (exhaustion of 802.11 retries) exceeding threshold(s
	EvTXFAIL_THRESH AsyncEventType = 108
	// Proximity Detection event.
	EvPROXD AsyncEventType = 109
	// AWDL RX Probe response.
	EvAWDL_RX_PRB_RESP AsyncEventType = 111
	// AWDL RX Action Frame
	EvAWDL_RX_ACT_FRAME AsyncEventType = 112
	// AWDL Wowl null
	EvAWDL_WOWL_NULLPKT AsyncEventType = 113
	// AWDL Phycal status.
	EvAWDL_PHYCAL_STATUS AsyncEventType = 114
	// AWDL OOB AF status.
	EvAWDL_OOB_AF_STATUS AsyncEventType = 115
	// Interleaved Scan status.
	EvAWDL_SCAN_STATUS AsyncEventType = 116
	// AWDL AW Start.
	EvAWDL_AW_START AsyncEventType = 117
	// AWDL AW End.
	EvAWDL_AW_END AsyncEventType = 118
	// AWDL AW Extension
	EvAWDL_AW_EXT             AsyncEventType = 119
	EvAWDL_PEER_CACHE_CONTROL AsyncEventType = 120
	EvCSA_START_IND           AsyncEventType = 121
	EvCSA_DONE_IND            AsyncEventType = 122
	EvCSA_FAILURE_IND         AsyncEventType = 123
	// CCA based channel quality repor
	EvCCA_CHAN_QUAL AsyncEventType = 124
	// to report change in BSSID while roaming.
	EvBSSID AsyncEventType = 125
	// tx error indication.
	EvTX_STAT_ERROR AsyncEventType = 126
	// credit check for BCMC supporte
	EvBCMC_CREDIT_SUPPORT AsyncEventType = 127
	// psta primary interface indication.
	EvPSTA_PRIMARY_INTF_IND AsyncEventType = 128
	// Handover Request Initiate
	EvBT_WIFI_HANDOVER_REQ AsyncEventType = 130
	// Southpaw TxInhibit notificatio
	EvSPW_TXINHIBIT AsyncEventType = 131
	// FBT Authentication Request Indication.
	EvFBT_AUTH_REQ_IND AsyncEventType = 132
	// Enhancement addition for RSS
	EvRSSI_LQM AsyncEventType = 133
	// Full probe/beacon (IEs etc) result
	EvPFN_GSCAN_FULL_RESULT AsyncEventType = 134
	// Significant change in rssi of bssids being tracked.s
	EvPFN_SWC AsyncEventType = 135
	// a STA been authroized for traffi
	EvAUTHORIZED AsyncEventType = 136
	// probe req with wl_event_rx_frame_data_t heade
	EvPROBREQ_MSG_RX AsyncEventType = 137
	// PFN completed scan of network lis
	EvPFN_SCAN_COMPLETE AsyncEventType = 138
	// RMC event.
	EvRMC_EVENT AsyncEventType = 139
	// DPSTA interface indicatio
	EvDPSTA_INTF_IND AsyncEventType = 140
	// RRM event.
	EvRRM AsyncEventType = 141
	// ULP entry event.
	EvULP AsyncEventType = 146
	// TCP Keep Alive Offload event.
	EvTKO AsyncEventType = 151
	// authentication request received.
	EvEXT_AUTH_REQ AsyncEventType = 187
	// authentication request received.
	EvEXT_AUTH_FRAME_RX AsyncEventType = 188
	// mgmt frame Tx complet
	EvMGMT_FRAME_TXSTATUS AsyncEventType = 189
	// highest val + 1 for range checkin
	evLAST AsyncEventType = 190
)

// EStatus represents the status field in async event messages.
// Reference: https://github.com/embassy-rs/embassy/blob/main/cyw43/src/consts.rs#L244-L279
type EStatus uint32

const (
	// EStatusSuccess indicates operation was successful.
	EStatusSuccess EStatus = 0
	// EStatusFail indicates operation failed.
	EStatusFail EStatus = 1
	// EStatusTimeout indicates operation timed out.
	EStatusTimeout EStatus = 2
	// EStatusNoNetworks indicates failed due to no matching network found.
	EStatusNoNetworks EStatus = 3
	// EStatusAbort indicates operation was aborted.
	EStatusAbort EStatus = 4
	// EStatusNoAck indicates protocol failure: packet not ack'd.
	EStatusNoAck EStatus = 5
	// EStatusUnsolicited indicates AUTH or ASSOC packet was unsolicited.
	// For PSK_SUP event, status=6 indicates successful key exchange (WLC_SUP_KEYED).
	EStatusUnsolicited EStatus = 6
	// EStatusAttempt indicates attempt to assoc to an auto auth configuration.
	EStatusAttempt EStatus = 7
	// EStatusPartial indicates scan results are incomplete.
	EStatusPartial EStatus = 8
	// EStatusNewscan indicates scan aborted by another scan.
	EStatusNewscan EStatus = 9
	// EStatusNewassoc indicates scan aborted due to assoc in progress.
	EStatusNewassoc EStatus = 10
	// EStatus11hQuiet indicates 802.11h quiet period started.
	EStatus11hQuiet EStatus = 11
	// EStatusSuppress indicates user disabled scanning (WLC_SET_SCANSUPPRESS).
	EStatusSuppress EStatus = 12
	// EStatusNochans indicates no allowable channels to scan.
	EStatusNochans EStatus = 13
	// EStatusCcxFastRoam indicates scan aborted due to CCX fast roam.
	EStatusCcxFastRoam EStatus = 14
	// EStatusCsAbort indicates abort channel select.
	EStatusCsAbort EStatus = 15
)
