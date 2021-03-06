package main

type Memory interface {
	Read(addr address) byte
	Write(addr address, data byte)
}

func ReadUint16(m Memory, addr address) uint16 {
	return uint16(m.Read(addr)) | (uint16(m.Read(addr+1)) << 8)
}

type CPUMemory struct {
	nes *Nes
}

func (*CPUMemory) Read(addr address) byte {
	// see https://wiki.nesdev.com/w/index.php/CPU_memory_map
	switch {
	case addr <= 0x1FFF:
		return nes.ram[addr&0x07FF]
	case addr <= 0x3FFF:
		return nes.ppu.ReadRegister(int(addr & 0x7))
	case addr == 0x4016:
		return nes.controller1.Read()
	case addr == 0x4017:
		return nes.controller2.Read()
	case addr <= 0x4017:
		// TODO NES APU and I/O REGISTERS
	case addr <= 0x401F:
		// CPU test mode
	case addr >= 0x4020:
		return nes.mapper.Read(addr)
	}
	return 0
}

func (*CPUMemory) Write(addr address, data byte) {
	// fmt.Println("mem write", addr, data)

	switch {
	case addr <= 0x1FFF:
		nes.ram[addr&0x07FF] = data
	case addr <= 0x3FFF:
		nes.ppu.WriteRegister(int(addr&0x7), data)
	case addr == 0x4014:
		// OAMDMA
		nes.ppu.WriteRegister(0x4014, data)
	case addr == 0x4016:
		nes.controller1.Write(data)
		nes.controller2.Write(data)
	case addr >= 0x4020:
		nes.mapper.Write(addr, data)
	}
	// TODO do the APU and I/O
}

type PPUMemory struct {
	nes *Nes
}

func (*PPUMemory) Read(addr address) byte {
	// https://wiki.nesdev.com/w/index.php/PPU_memory_map
	addr = addr & 0x3FFF
	switch {
	case addr <= 0x2FFF:
		return nes.mapper.Read(addr)
	case addr <= 0x3EFF:
		// mirrored from 0x2000
		return nes.mapper.Read(addr - 0x1000)
	case addr <= 0x3FFF:
		// (only bottom 0x1F -- 5 bits)
		index := addr & 0x1F
		return nes.ppu.palette[index]
	}
	return 0 // can't be reached
}

func (*PPUMemory) Write(addr address, data byte) {
	addr = addr & 0x3FFF
	switch {
	case addr <= 0x2FFF:
		nes.mapper.Write(addr, data)
	case addr <= 0x3EFF:
		// mirrored from 0x2000
		nes.mapper.Write(addr-0x1000, data)
	case addr <= 0x3FFF:
		index := addr & 0x1F
		if index == 0x10 || index == 0x14 || index == 0x18 || index == 0x1C {
			index -= 0x10
		}
		nes.ppu.palette[index] = data
	}
}
