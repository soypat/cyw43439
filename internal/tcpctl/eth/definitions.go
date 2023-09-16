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
//go:generate stringer -type=DHCPOption -trimprefix=DHCP_
const (
	DHCP_WordAligned                 DHCPOption = 0
	DHCP_SubnetMask                  DHCPOption = 1
	DHCP_TimeOffset                  DHCPOption = 2  // Time offset in seconds from UTC
	DHCP_Router                      DHCPOption = 3  // N/4 router addresses
	DHCP_TimeServers                 DHCPOption = 4  // N/4 time server addresses
	DHCP_NameServers                 DHCPOption = 5  // N/4 IEN-116 server addresses
	DHCP_DNSServers                  DHCPOption = 6  // N/4 DNS server addresses
	DHCP_LogServers                  DHCPOption = 7  // N/4 logging server addresses
	DHCP_CookieServers               DHCPOption = 8  // N/4 quote server addresses
	DHCP_LPRServers                  DHCPOption = 9  // N/4 printer server addresses
	DHCP_ImpressServers              DHCPOption = 10 // N/4 impress server addresses
	DHCP_RLPServers                  DHCPOption = 11 // N/4 RLP server addresses
	DHCP_HostName                    DHCPOption = 12 // Hostname string
	DHCP_BootFileSize                DHCPOption = 13 // Size of boot file in 512 byte chunks
	DHCP_MeritDumpFile               DHCPOption = 14 // Client to dump and name of file to dump to
	DHCP_DomainName                  DHCPOption = 15 // The DNS domain name of the client
	DHCP_SwapServer                  DHCPOption = 16 // Swap server addresses
	DHCP_RootPath                    DHCPOption = 17 // Path name for root disk
	DHCP_ExtensionFile               DHCPOption = 18 // Patch name for more BOOTP info
	DHCP_IPLayerForwarding           DHCPOption = 19 // Enable or disable IP forwarding
	DHCP_Srcrouteenabler             DHCPOption = 20 // Enable or disable source routing
	DHCP_PolicyFilter                DHCPOption = 21 // Routing policy filters
	DHCP_MaximumDGReassemblySize     DHCPOption = 22 // Maximum datagram reassembly size
	DHCP_DefaultIPTTL                DHCPOption = 23 // Default IP time-to-live
	DHCP_PathMTUAgingTimeout         DHCPOption = 24 // Path MTU aging timeout
	DHCP_MTUPlateau                  DHCPOption = 25 // Path MTU plateau table
	DHCP_InterfaceMTUSize            DHCPOption = 26 // Interface MTU size
	DHCP_AllSubnetsAreLocal          DHCPOption = 27 // All subnets are local
	DHCP_BroadcastAddress            DHCPOption = 28 // Broadcast address
	DHCP_PerformMaskDiscovery        DHCPOption = 29 // Perform mask discovery
	DHCP_ProvideMasktoOthers         DHCPOption = 30 // Provide mask to others
	DHCP_PerformRouterDiscovery      DHCPOption = 31 // Perform router discovery
	DHCP_RouterSolicitationAddress   DHCPOption = 32 // Router solicitation address
	DHCP_StaticRoutingTable          DHCPOption = 33 // Static routing table
	DHCP_TrailerEncapsulation        DHCPOption = 34 // Trailer encapsulation
	DHCP_ARPCacheTimeout             DHCPOption = 35 // ARP cache timeout
	DHCP_EthernetEncapsulation       DHCPOption = 36 // Ethernet encapsulation
	DHCP_DefaultTCPTimetoLive        DHCPOption = 37 // Default TCP time to live
	DHCP_TCPKeepaliveInterval        DHCPOption = 38 // TCP keepalive interval
	DHCP_TCPKeepaliveGarbage         DHCPOption = 39 // TCP keepalive garbage
	DHCP_NISDomainName               DHCPOption = 40 // NIS domain name
	DHCP_NISServerAddresses          DHCPOption = 41 // NIS server addresses
	DHCP_NTPServersAddresses         DHCPOption = 42 // NTP servers addresses
	DHCP_VendorSpecificInformation   DHCPOption = 43 // Vendor specific information
	DHCP_NetBIOSNameServer           DHCPOption = 44 // NetBIOS name server
	DHCP_NetBIOSDatagramDistribution DHCPOption = 45 // NetBIOS datagram distribution
	DHCP_NetBIOSNodeType             DHCPOption = 46 // NetBIOS node type
	DHCP_NetBIOSScope                DHCPOption = 47 // NetBIOS scope
	DHCP_XWindowFontServer           DHCPOption = 48 // X window font server
	DHCP_XWindowDisplayManager       DHCPOption = 49 // X window display manager
	DHCP_RequestedIPaddress          DHCPOption = 50 // Requested IP address
	DHCP_IPAddressLeaseTime          DHCPOption = 51 // IP address lease time
	DHCP_OptionOverload              DHCPOption = 52 // Overload “sname” or “file”
	DHCP_MessageType                 DHCPOption = 53 // DHCP message type
	DHCP_ServerIdentification        DHCPOption = 54 // DHCP server identification
	DHCP_ParameterRequestList        DHCPOption = 55 // Parameter request list
	DHCP_Message                     DHCPOption = 56 // DHCP error message
	DHCP_MaximumMessageSize          DHCPOption = 57 // DHCP maximum message size
	DHCP_RenewTimeValue              DHCPOption = 58 // DHCP renewal (T1) time
	DHCP_RebindingTimeValue          DHCPOption = 59 // DHCP rebinding (T2) time
	DHCP_ClientIdentifier            DHCPOption = 60 // Client identifier
	DHCP_ClientIdentifier1           DHCPOption = 61 // Client identifier
)
