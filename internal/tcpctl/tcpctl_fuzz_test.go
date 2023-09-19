package tcpctl

import "testing"

type socketEth interface {
	HandleEth([]byte) (int, error)
	IsPendingHandling() bool
	NeedsHandling() bool
}

func FuzzSocket(f *testing.F) {
	defaultCfg := StackConfig{
		MAC:         nil, // Accept all incoming packets.
		IP:          nil, // Accept all incoming packets.
		MaxUDPConns: 2,
		MaxTCPConns: 2,
	}
	for _, packet := range testTCPPackets {
		f.Add(2, 2, packet)
	}
	for _, packet := range interestingPackets {
		f.Add(2, 2, packet)
	}
	f.Add(2, 2, testUDPPacket)
	var buf [_MTU]byte
	f.Fuzz(func(t *testing.T, nTCP, nUDP int, eth []byte) {
		if uint(nTCP) > 3 || uint(nUDP) > 3 {
			return
		}
		cfg := defaultCfg
		if len(eth) > 0 {
			cfg.MaxTCPConns = nTCP
			cfg.MaxUDPConns = nUDP
		}
		s := NewStack(cfg)

		s.OpenTCP(80, func([]byte, *TCPPacket) (int, error) {
			return nUDP, nil
		})
		s.OpenUDP(80, func([]byte, *UDPPacket) (int, error) {
			return nTCP, nil
		})
		defer func() {
			if s.pendingTCPv4 != 0 || s.pendingUDPv4 != 0 {
				panic(s)
			}
			s.CloseTCP(80)
			s.CloseUDP(80)
			if s.pendingTCPv4 != 0 || s.pendingUDPv4 != 0 {
				panic(s)
			}
		}()
		err := s.RecvEth(eth)
		if err != nil {
			return
		}
		s.HandleEth(buf[:])
	})
}

var testUDPPacket = []byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x78, 0x44, 0x76, 0xc4, 0x8d, 0xb0, 0x08, 0x00, 0x45, 0x00, // |......xDv.....E.|
	0x00, 0xa2, 0x4a, 0xb0, 0x00, 0x00, 0x80, 0x11, 0x6c, 0xdc, 0xc0, 0xa8, 0x00, 0x6f, 0xc0, 0xa8, // |..J.....l....o..|
	0x00, 0xff, 0x44, 0x5c, 0x44, 0x5c, 0x00, 0x8e, 0x27, 0x8f, 0x7b, 0x22, 0x76, 0x65, 0x72, 0x73, // |..D\D\..'.{"vers|
	0x69, 0x6f, 0x6e, 0x22, 0x3a, 0x20, 0x5b, 0x32, 0x2c, 0x20, 0x30, 0x5d, 0x2c, 0x20, 0x22, 0x70, // |ion": [2, 0], "p|
	0x6f, 0x72, 0x74, 0x22, 0x3a, 0x20, 0x31, 0x37, 0x35, 0x30, 0x30, 0x2c, 0x20, 0x22, 0x68, 0x6f, // |ort": 17500, "ho|
	0x73, 0x74, 0x5f, 0x69, 0x6e, 0x74, 0x22, 0x3a, 0x20, 0x31, 0x38, 0x31, 0x32, 0x36, 0x35, 0x36, // |st_int": 1812656|
	0x30, 0x39, 0x32, 0x35, 0x37, 0x34, 0x32, 0x31, 0x30, 0x34, 0x36, 0x37, 0x33, 0x33, 0x36, 0x32, // |0925742104673362|
	0x36, 0x31, 0x37, 0x33, 0x32, 0x31, 0x37, 0x30, 0x35, 0x37, 0x36, 0x33, 0x34, 0x37, 0x39, 0x33, // |6173217057634793|
	0x2c, 0x20, 0x22, 0x64, 0x69, 0x73, 0x70, 0x6c, 0x61, 0x79, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x3a, // |, "displayname":|
	0x20, 0x22, 0x22, 0x2c, 0x20, 0x22, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x73, // | "", "namespaces|
	0x22, 0x3a, 0x20, 0x5b, 0x38, 0x31, 0x35, 0x32, 0x34, 0x36, 0x32, 0x30, 0x30, 0x30, 0x5d, 0x7d, // |": [8152462000]}|
}

var testTCPPackets = [][]byte{
	{
		0xd8, 0x5e, 0xd3, 0x43, 0x03, 0xeb, 0xec, 0x08, 0x6b, 0x3f, 0x2a, 0xce, 0x08, 0x00, 0x45, 0x00,
		0x00, 0x34, 0xc6, 0x36, 0x40, 0x00, 0x40, 0x06, 0xf0, 0xa8, 0xc0, 0xa8, 0x01, 0x01, 0xc0, 0xa8,
		0x01, 0x93, 0x00, 0x50, 0xc4, 0x80, 0x27, 0x40, 0xf4, 0xcb, 0x16, 0x5d, 0xc6, 0xef, 0x80, 0x11,
		0x1d, 0x54, 0xc9, 0x29, 0x00, 0x00, 0x01, 0x01, 0x08, 0x0a, 0x01, 0xad, 0x35, 0x67, 0x17, 0x1e,
		0xff, 0xfd,
	},
	{ // SYN
		0xec, 0x08, 0x6b, 0x3f, 0x2a, 0xce, 0xd8, 0x5e, 0xd3, 0x43, 0x03, 0xeb, 0x08, 0x00, 0x45, 0x00,
		0x00, 0x3c, 0x91, 0x60, 0x40, 0x00, 0x40, 0x06, 0x25, 0x77, 0xc0, 0xa8, 0x01, 0x93, 0xc0, 0xa8,
		0x01, 0x01, 0xc4, 0x80, 0x00, 0x50, 0x16, 0x5d, 0xc5, 0x65, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x02,
		0xfa, 0xf0, 0x11, 0x7c, 0x00, 0x00, 0x02, 0x04, 0x05, 0xb4, 0x04, 0x02, 0x08, 0x0a, 0x17, 0x1e,
		0xff, 0xfc, 0x00, 0x00, 0x00, 0x00, 0x01, 0x03, 0x03, 0x07,
	},
	{ // SYN,ACK
		0xd8, 0x5e, 0xd3, 0x43, 0x03, 0xeb, 0xec, 0x08, 0x6b, 0x3f, 0x2a, 0xce, 0x08, 0x00, 0x45, 0x00,
		0x00, 0x3c, 0x00, 0x00, 0x40, 0x00, 0x40, 0x06, 0xb6, 0xd7, 0xc0, 0xa8, 0x01, 0x01, 0xc0, 0xa8,
		0x01, 0x93, 0x00, 0x50, 0xc4, 0x80, 0x27, 0x40, 0xf3, 0x50, 0x16, 0x5d, 0xc5, 0x66, 0xa0, 0x12,
		0x71, 0x20, 0x49, 0x9d, 0x00, 0x00, 0x02, 0x04, 0x05, 0xb4, 0x04, 0x02, 0x08, 0x0a, 0x01, 0xad,
		0x35, 0x65, 0x17, 0x1e, 0xff, 0xfc, 0x01, 0x03, 0x03, 0x02,
	},
	{ // ACK
		0xd8, 0x5e, 0xd3, 0x43, 0x03, 0xeb, 0xec, 0x08, 0x6b, 0x3f, 0x2a, 0xce, 0x08, 0x00, 0x45, 0x00,
		0x00, 0x34, 0xc6, 0x34, 0x40, 0x00, 0x40, 0x06, 0xf0, 0xaa, 0xc0, 0xa8, 0x01, 0x01, 0xc0, 0xa8,
		0x01, 0x93, 0x00, 0x50, 0xc4, 0x80, 0x27, 0x40, 0xf3, 0x51, 0x16, 0x5d, 0xc6, 0xef, 0x80, 0x10,
		0x1d, 0x54, 0xca, 0xa6, 0x00, 0x00, 0x01, 0x01, 0x08, 0x0a, 0x01, 0xad, 0x35, 0x65, 0x17, 0x1e,
		0xff, 0xfd,
	},
	{ // PSH,ACK
		0xd8, 0x5e, 0xd3, 0x43, 0x03, 0xeb, 0xec, 0x08, 0x6b, 0x3f, 0x2a, 0xce, 0x08, 0x00, 0x45, 0x00,
		0x01, 0xae, 0xc6, 0x35, 0x40, 0x00, 0x40, 0x06, 0xef, 0x2f, 0xc0, 0xa8, 0x01, 0x01, 0xc0, 0xa8,
		0x01, 0x93, 0x00, 0x50, 0xc4, 0x80, 0x27, 0x40, 0xf3, 0x51, 0x16, 0x5d, 0xc6, 0xef, 0x80, 0x18,
		0x1d, 0x54, 0x9c, 0xcf, 0x00, 0x00, 0x01, 0x01, 0x08, 0x0a, 0x01, 0xad, 0x35, 0x67, 0x17, 0x1e,
		0xff, 0xfd, 0x48, 0x54, 0x54, 0x50, 0x2f, 0x31, 0x2e, 0x31, 0x20, 0x32, 0x30, 0x30, 0x20, 0x4f,
		0x6b, 0x0d, 0x0a, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x2d, 0x54, 0x79, 0x70, 0x65, 0x3a,
		0x20, 0x74, 0x65, 0x78, 0x74, 0x2f, 0x68, 0x74, 0x6d, 0x6c, 0x0d, 0x0a, 0x53, 0x65, 0x72, 0x76,
		0x65, 0x72, 0x3a, 0x20, 0x68, 0x74, 0x74, 0x70, 0x64, 0x0d, 0x0a, 0x44, 0x61, 0x74, 0x65, 0x3a,
		0x20, 0x53, 0x75, 0x6e, 0x2c, 0x20, 0x30, 0x34, 0x20, 0x4a, 0x61, 0x6e, 0x20, 0x31, 0x39, 0x37,
		0x30, 0x20, 0x30, 0x36, 0x3a, 0x31, 0x33, 0x3a, 0x30, 0x36, 0x20, 0x47, 0x4d, 0x54, 0x0d, 0x0a,
		0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x3a, 0x20, 0x63, 0x6c, 0x6f, 0x73,
		0x65, 0x0d, 0x0a, 0x43, 0x61, 0x63, 0x68, 0x65, 0x2d, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x6f, 0x6c,
		0x3a, 0x20, 0x6e, 0x6f, 0x2d, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2c, 0x20, 0x6e, 0x6f, 0x2d, 0x63,
		0x61, 0x63, 0x68, 0x65, 0x2c, 0x20, 0x6d, 0x75, 0x73, 0x74, 0x2d, 0x72, 0x65, 0x76, 0x61, 0x6c,
		0x69, 0x64, 0x61, 0x74, 0x65, 0x0d, 0x0a, 0x43, 0x61, 0x63, 0x68, 0x65, 0x2d, 0x43, 0x6f, 0x6e,
		0x74, 0x72, 0x6f, 0x6c, 0x3a, 0x20, 0x70, 0x6f, 0x73, 0x74, 0x2d, 0x63, 0x68, 0x65, 0x63, 0x6b,
		0x3d, 0x30, 0x2c, 0x20, 0x70, 0x72, 0x65, 0x2d, 0x63, 0x68, 0x65, 0x63, 0x6b, 0x3d, 0x30, 0x0d,
		0x0a, 0x50, 0x72, 0x61, 0x67, 0x6d, 0x61, 0x3a, 0x20, 0x6e, 0x6f, 0x2d, 0x63, 0x61, 0x63, 0x68,
		0x65, 0x0d, 0x0a, 0x43, 0x61, 0x63, 0x68, 0x65, 0x2d, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x6f, 0x6c,
		0x3a, 0x20, 0x6e, 0x6f, 0x2d, 0x63, 0x61, 0x63, 0x68, 0x65, 0x0d, 0x0a, 0x50, 0x72, 0x61, 0x67,
		0x6d, 0x61, 0x3a, 0x20, 0x6e, 0x6f, 0x2d, 0x63, 0x61, 0x63, 0x68, 0x65, 0x0d, 0x0a, 0x45, 0x78,
		0x70, 0x69, 0x72, 0x65, 0x73, 0x3a, 0x20, 0x30, 0x0d, 0x0a, 0x0d, 0x0a, 0x7b, 0x75, 0x70, 0x74,
		0x69, 0x6d, 0x65, 0x3a, 0x3a, 0x20, 0x30, 0x37, 0x3a, 0x31, 0x33, 0x3a, 0x30, 0x36, 0x20, 0x75,
		0x70, 0x20, 0x33, 0x20, 0x64, 0x61, 0x79, 0x73, 0x2c, 0x20, 0x20, 0x36, 0x3a, 0x31, 0x33, 0x2c,
		0x20, 0x20, 0x6c, 0x6f, 0x61, 0x64, 0x20, 0x61, 0x76, 0x65, 0x72, 0x61, 0x67, 0x65, 0x3a, 0x20,
		0x30, 0x2e, 0x30, 0x34, 0x2c, 0x20, 0x30, 0x2e, 0x31, 0x30, 0x2c, 0x20, 0x30, 0x2e, 0x30, 0x36,
		0x7d, 0x7b, 0x69, 0x70, 0x69, 0x6e, 0x66, 0x6f, 0x3a, 0x3a, 0x26, 0x6e, 0x62, 0x73, 0x70, 0x3b,
		0x49, 0x50, 0x3a, 0x20, 0x30, 0x2e, 0x30, 0x2e, 0x30, 0x2e, 0x30, 0x7d,
	},
}

var interestingPackets = [][]byte{
	[]byte("0"),
	[]byte("000000000000\b\x00E0\x00<00000\x0600\xc0\xa8\x01\x93\xc0\xa8\x01\x01Ā\x00P\x16]\xc5e\x00\x00\x00\x00\xa0\x02\xfa\xf0\x11|\x00\x00\x02\x04\x05\xf1\xf1\xf1\xf1\xf1\xf1\xf1\xff\xfc\x00\x00\x00\x00\x01\x03\x03\a"),
	[]byte("000000000000\b\x00E0\x00C0000000000000000"),
	[]byte("000000000000\b\x00E0\x00\xa200000\x1100\xc0\xa8\x00x\xc0\xa8\x00\xffDXDX\x00\x8e00{0vXrxion : [0, 0X,0\"aoxt 70377730  dxsalayxaae0: \" , \"a:0105000  hosx_Znx\"7 081275707277221076730626177207057604797, \"aixpxaxnXma\"7 0"),
	[]byte("0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00\x1900000\x06000000000000000"),
	[]byte("000000000000\b\x000000 0\x00\x0000\x000\x02\x04\x0500\x02\b0"),
	[]byte("000000000000\b\x00E0\x01100000\x060000000000000000000000X0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x1100000000000000\x00\x040000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00!00000\x1100000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x1100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x1100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00A0000000000000000000"),
	[]byte("000000000000\b\x00E0\x00 000000000000000000000000000000000"),
	[]byte("000000000000\b\x00G0\x00 00000\x1100000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x06000000000000000000000000000000000000000000"),
	[]byte("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00<00000\x0600\xc0\xa8\x01\x93\xc0\xa8\x01\x01Ā\x00P\x16]\xc5e\x00\x00\x00\x00\xa0\x02\xfa\xf0\x11|\x00\x00\x02\x04\x05\xf1\xf1\xf1\xf1\xf1\xf1\xf1\xff\xfc\x00\x00\x00\x00\x01\x03\x03\a"),
	[]byte("0000000000000000000000000000000000"),
	[]byte("000000000000\b\x000\x00\x00\x00 0\x00\x00\x00\x00\x00\x00\x02\x04\x0500\x02\b0"),
	[]byte("000000000000\b\x00E0\x00000000\x110000000000\x00\x0000000000000000000000000000"),
	[]byte("\xd8^\xd3C\x03\xeb\xec\bk?*\xce\b\x00E\x00\x004\xc66@\x00@\x06\xf0\xa8\xc0\xa8\x01\x01\xc0\xa8\x01\x93\x00PĀ'@\xf4\xcb\x16]\xc6\xef\x80\x11\x1dT\xc9)\x00\x00\x01\x01\b\n\x01\xad5g\x17\x1e\xff\xfd"),
	[]byte("000000000000\b\x00E0\x01100000\x060000000000000000000000X0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000@>\xb5\x10\x92\x03\xa4\x8f00000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x060000000000000000000000X000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x06000000000000\x00\x000000000000000000000000000000"),
	[]byte("000000000000\b\x00G0\x02\x1c00000\x1100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x000\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x02\x04\x0500\x02\b\x00"),
	[]byte("000000000000\b\x00E0\x00\xa200000\x1100\xc0\xa8\x00o\xc0\xa8\x00\xffD\\D\\\x00\x8e'\x8f{\"version\": [2, 0], \"port\": 17500, \"host_int\": 181265609257421046733626173217057634793, \"displa}name\": \"\", \"namespaces\": [8152462000]y"),
	[]byte("000000000000\b\x00E0\x00<00000\x0600\xc0\xa8\x01\x93\xc0\xa8\x01\x01Ā\x00P\x16]\xc5e\x00\x00\x00\x00\xa0\x02\xfa\xf0\x11|\x00\x00\x02\x04\x05\xf1\xf1\xf1\xf1\xf1\xf1\xf1\xff\xfc\x00\x00\x00\x00\x01\x03\x03\a"),
	[]byte("000000000000\b\x00E0\x01\xae00000\x0600\xc0\xa8\x010\xc0x\x01\x93\x00X\xc4x'A\xf3X\x16X\xc6\xef\x800\x1d8\x9c\xcf\x00 \x01\x01\b\n\x01x5a\x170\xff\xfdHATX/0.0 000 Xk0\n0oXtYnx-Xyxe0 xext0hxma\r0SArxex:0hXtxd0\n0aae7 Xux,000 0ax 0900000:03001 XMC\r0CXnaeatxoa:0cZoxe0\n0aXha-AoxtxoX:0na-xtxrX,0na-aaahx,0mXsx-xexaYixaae0\n0aXha-Aoxtxoa:0pZsx-ahccx=0,0pXe0cbeckA00\n0rZgxa0 xo0cXcce0\n0aXha-Boxtxoa:0nZ-cachx\r0P9axmX:0nX-bachx\r0EApxrXs7 0\r0\r {8pxixe0:000:03007 xp030dAyx,0 0:030 0lXaX avaragc:00000,00010,00000}Xixiafx:0&xbxp0IX:1\x00A\x0600000"),
	[]byte("000000000000\b\x0000000000000000000000"),
	[]byte("000000000000\b\x00E0\x00000000\x06000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x04000000\x06000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x06\x0600000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00000000000000000xxxx0"),
	[]byte("000000000000\b\x00E0\x00 00000\x0600000000000000000000000000"),
	[]byte("000000000000\b\x00E0\x01100000\x060000000000000000000000X0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000@>\xb5\x10\x92\x03\xa4\x8f00000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x00G0\x02 00000\x11000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
	[]byte("000000000000\b\x000\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x02\x04\x05\x00\x01\x02\b\x00"),
}
