package eth

// Ethertype values. From: http://en.wikipedia.org/wiki/Ethertype
//
//go:generate stringer -type=EtherType -trimprefix=EtherType
const (
	EtherTypeIPv4                EtherType = 0x0800
	EtherTypeARP                 EtherType = 0x0806
	EtherTypeWakeOnLAN           EtherType = 0x0842
	EtherTypeTRILL               EtherType = 0x22F3
	EtherTypeDECnetPhase4        EtherType = 0x6003
	EtherTypeRARP                EtherType = 0x8035
	EtherTypeAppleTalk           EtherType = 0x809B
	EtherTypeAARP                EtherType = 0x80F3
	EtherTypeIPX1                EtherType = 0x8137
	EtherTypeIPX2                EtherType = 0x8138
	EtherTypeQNXQnet             EtherType = 0x8204
	EtherTypeIPv6                EtherType = 0x86DD
	EtherTypeEthernetFlowControl EtherType = 0x8808
	EtherTypeIEEE802_3           EtherType = 0x8809
	EtherTypeCobraNet            EtherType = 0x8819
	EtherTypeMPLSUnicast         EtherType = 0x8847
	EtherTypeMPLSMulticast       EtherType = 0x8848
	EtherTypePPPoEDiscovery      EtherType = 0x8863
	EtherTypePPPoESession        EtherType = 0x8864
	EtherTypeJumboFrames         EtherType = 0x8870
	EtherTypeHomePlug1_0MME      EtherType = 0x887B
	EtherTypeIEEE802_1X          EtherType = 0x888E
	EtherTypePROFINET            EtherType = 0x8892
	EtherTypeHyperSCSI           EtherType = 0x889A
	EtherTypeAoE                 EtherType = 0x88A2
	EtherTypeEtherCAT            EtherType = 0x88A4
	EtherTypeEthernetPowerlink   EtherType = 0x88AB
	EtherTypeLLDP                EtherType = 0x88CC
	EtherTypeSERCOS3             EtherType = 0x88CD
	EtherTypeHomePlugAVMME       EtherType = 0x88E1
	EtherTypeMRP                 EtherType = 0x88E3
	EtherTypeIEEE802_1AE         EtherType = 0x88E5
	EtherTypeIEEE1588            EtherType = 0x88F7
	EtherTypeIEEE802_1ag         EtherType = 0x8902
	EtherTypeFCoE                EtherType = 0x8906
	EtherTypeFCoEInit            EtherType = 0x8914
	EtherTypeRoCE                EtherType = 0x8915
	EtherTypeCTP                 EtherType = 0x9000
	EtherTypeVeritasLLT          EtherType = 0xCAFE
	EtherTypeVLAN                EtherType = 0x8100
	EtherTypeServiceVLAN         EtherType = 0x88a8
	// minEthPayload is the minimum payload size for an Ethernet frame, assuming
	// that no 802.1Q VLAN tags are present.
	minEthPayload = 46
)

type DHCPOption uint8

// DHCP options. Taken from https://help.sonicwall.com/help/sw/eng/6800/26/2/3/content/Network_DHCP_Server.042.12.htm.
//
//go:generate stringer -type=DHCPOption -trimprefix=DHCP
const (
	DHCPWordAligned                 DHCPOption = 0
	DHCPSubnetMask                  DHCPOption = 1
	DHCPTimeOffset                  DHCPOption = 2
	DHCPRouter                      DHCPOption = 3
	DHCPTimeServers                 DHCPOption = 4
	DHCPNameServers                 DHCPOption = 5
	DHCPDNSServers                  DHCPOption = 6
	DHCPLogServers                  DHCPOption = 7
	DHCPCookieServers               DHCPOption = 8
	DHCPLPRServers                  DHCPOption = 9
	DHCPImpressServers              DHCPOption = 10
	DHCPRLPServers                  DHCPOption = 11
	DHCPHostName                    DHCPOption = 12
	DHCPBootFileSize                DHCPOption = 13
	DHCPMeritDumpFile               DHCPOption = 14
	DHCPDomainName                  DHCPOption = 15
	DHCPSwapServer                  DHCPOption = 16
	DHCPRootPath                    DHCPOption = 17
	DHCPExtensionFile               DHCPOption = 18
	DHCPIPLayerForwarding           DHCPOption = 19
	DHCPSrcrouteenabler             DHCPOption = 20
	DHCPPolicyFilter                DHCPOption = 21
	DHCPMaximumDGReassemblySize     DHCPOption = 22
	DHCPDefaultIPTTL                DHCPOption = 23
	DHCPPathMTUAgingTimeout         DHCPOption = 24
	DHCPMTUPlateau                  DHCPOption = 25
	DHCPInterfaceMTUSize            DHCPOption = 26
	DHCPAllSubnetsAreLocal          DHCPOption = 27
	DHCPBroadcastAddress            DHCPOption = 28
	DHCPPerformMaskDiscovery        DHCPOption = 29
	DHCPProvideMasktoOthers         DHCPOption = 30
	DHCPPerformRouterDiscovery      DHCPOption = 31
	DHCPRouterSolicitationAddress   DHCPOption = 32
	DHCPStaticRoutingTable          DHCPOption = 33
	DHCPTrailerEncapsulation        DHCPOption = 34
	DHCPARPCacheTimeout             DHCPOption = 35
	DHCPEthernetEncapsulation       DHCPOption = 36
	DHCPDefaultTCPTimetoLive        DHCPOption = 37
	DHCPTCPKeepaliveInterval        DHCPOption = 38
	DHCPTCPKeepaliveGarbage         DHCPOption = 39
	DHCPNISDomainName               DHCPOption = 40
	DHCPNISServerAddresses          DHCPOption = 41
	DHCPNTPServersAddresses         DHCPOption = 42
	DHCPVendorSpecificInformation   DHCPOption = 43
	DHCPNetBIOSNameServer           DHCPOption = 44
	DHCPNetBIOSDatagramDistribution DHCPOption = 45
	DHCPNetBIOSNodeType             DHCPOption = 46
	DHCPNetBIOSScope                DHCPOption = 47
	DHCPXWindowFontServer           DHCPOption = 48
	DHCPXWindowDisplayManager       DHCPOption = 49
	DHCPRequestedIPaddress          DHCPOption = 50
	DHCPIPAddressLeaseTime          DHCPOption = 51
	DHCPOptionOverload              DHCPOption = 52
	DHCPDHCPMessageType             DHCPOption = 53
	DHCPDHCPServerIdentification    DHCPOption = 54
	DHCPParameterRequestList        DHCPOption = 55
	DHCPMessage                     DHCPOption = 56
	DHCPMaximumMessageSize          DHCPOption = 57
	DHCPRenewTimeValue              DHCPOption = 58
	DHCPRebindingTimeValue          DHCPOption = 59
	DHCPClientIdentifier            DHCPOption = 60
	DHCPClientIdentifier1           DHCPOption = 61
)
