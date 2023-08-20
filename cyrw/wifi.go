package cyrw

import "github.com/soypat/cyw43439/whd"

func (d *Device) initControl(clm string) error {
	const chunkSize = 1024
	remaining := clm
	offset := 0
	var buf8 [chunkSize]byte

	for len(remaining) > 0 {
		chunk := remaining[offset:min(len(remaining), chunkSize)]
		remaining = remaining[len(chunk):]
		var flag uint16 = 0x1000 // Download flag handler version.
		if offset == 0 {
			flag |= 0x0002 // Flag begin.
		}
		offset += len(chunk)
		if offset == len(clm) {
			flag |= 0x0004 // Flag end.
		}
		header := whd.DownloadHeader{ // No CRC.
			Flags: flag,
			Type:  2, // CLM download type.
			Len:   uint32(len(chunk)),
		}
		n := copy(buf8[:8], "clmload\x00")
		header.Put(buf8[8:20])
		n += whd.DL_HEADER_LEN
		n += copy(buf8[20:], chunk)

		err := d.doIoctlSet(whd.WLC_SET_VAR, whd.WWD_STA_INTERFACE, buf8[:n])
		if err != nil {
			return err
		}
	}

	return nil
}
